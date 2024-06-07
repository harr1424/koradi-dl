package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
)

func main() {
	output := termenv.NewOutput(os.Stdout)
	msg := output.String("Welcome to the Koradi Archive Utility\n").
		Bold().
		Underline()
	fmt.Println(msg)

	ensureElevatedPrivileges()
	confirmWorkingDir()

	initialModel := newModel()
	p := tea.NewProgram(initialModel)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		run(output, p)
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Press any key to exit...")
	bufio.NewReader(os.Stdin).ReadRune()
}

func ensureElevatedPrivileges() {
	if runtime.GOOS == "windows" {
		checkWindowsPrivileges()
	} else {
		checkUnixPrivileges()
	}
}

func checkWindowsPrivileges() {
	out, err := exec.Command("net", "session").Output()
	if err != nil {
		log.Fatal("Unable to check for sufficient privileges. Terminating: ", err)
	}

	if string(out) == "" {
		fmt.Println("This program requires elevated privileges in order to create directories and files on your computer. You will be prompted to enter your password.")
		cmd := exec.Command("powershell", "Start-Process", "cmd.exe", "/c", os.Args[0], "-Verb", "runAs")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			log.Fatal("Unable to run as admin. Terminating: ", err)
		}
		os.Exit(0)
	}
}

func checkUnixPrivileges() {
	if os.Geteuid() != 0 {
		fmt.Println("This program requires elevated privileges in order to create directories and files on your computer. You will be prompted to enter your password:")
		cmd := exec.Command("sudo", os.Args[0])
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			log.Fatal("Unable to run as root. Terminating: ", err)
		}
		os.Exit(0)
	}
}

func confirmWorkingDir() {
	exPath, err := os.Executable()
	if err != nil {
		log.Fatal("Unable to detect working directory")
	}
	exDir := filepath.Dir(exPath)
	if err := os.Chdir(exDir); err != nil {
		log.Fatal("Unable to change working directory", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal("Unable to detect working directory:", err)
	}
	fmt.Printf("Files will be downloaded to:  %s\n", wd)
	fmt.Println("Please confirm that you have 100GB free at this location by entering 'Y' to continue or any other key to quit:")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal("Error reading input:", err)
	}

	input = strings.TrimSpace(input)

	if strings.ToUpper(input) == "Y" {
		return
	} else {
		log.Fatal("User exited program")
	}
}

func run(output *termenv.Output, p *tea.Program) {
	var new_downloads []string
	var errors_ocurred []string
	var mu sync.Mutex

	p.Send(logMsg{Content: output.String("Searching for available downloads...\n")})

	urls := [6]string{
		"https://koradi.org/en/downloads/",
		"https://koradi.org/es/descargas/",
		"https://koradi.org/fr/telechargements/",
		"https://koradi.org/po/downloads/",
		"https://koradi.org/it/download/",
		"https://koradi.org/de/herunterladen/",
	}

	var pages = map[int][]string{
		0: {}, // en
		1: {}, // es
		2: {}, // fr
		3: {}, // po
		4: {}, // it
		5: {}, // de
	}

	var lang_wg sync.WaitGroup
	lang_wg.Add(len(urls))
	for i, v := range urls {
		go func(i int, v string) {
			defer lang_wg.Done()
			links, err := scrape_authors(v)
			if err != nil {
				p.Send(logMsg{Content: output.String(fmt.Sprintf("Error scraping authors from %s: %v", v, err)).Foreground(output.Color("1"))})
				mu.Lock()
				errors_ocurred = append(errors_ocurred, err.Error())
				mu.Unlock()
				return
			}
			mu.Lock()
			pages[i] = links
			mu.Unlock()
			p.Send(logMsg{Content: output.String(fmt.Sprintf("Scraped authors from %s successfully", v)).Foreground(output.Color("34"))})
		}(i, v)
	}
	lang_wg.Wait()

	p.Send(logMsg{Content: output.String("Finished scraping authors").Bold().Underline()})

	var pages_wg sync.WaitGroup
	pages_wg.Add(len(pages))
	for i, lang := range pages {
		go func(i int, lang []string) {
			defer pages_wg.Done()
			var lang_zips []string

			p.Send(logMsg{Content: output.String(fmt.Sprintf("Checking %v %v links for .zip files...\n", len(lang), get_lang(i))).Bold().Underline()})

			downloadDir := get_lang(i)
			if err := os.MkdirAll(downloadDir, 0755); err != nil {
				p.Send(logMsg{Content: output.String(fmt.Sprintf("Terminating because a directory could not be created: %v", err)).Foreground(output.Color("1"))})
				return
			}

			for _, author := range lang {
				if strings.Contains(author, "/"+get_lang(i)+"/") {
					p.Send(logMsg{Content: output.String(fmt.Sprintf("Found %s\n", author))})
					zips, err := scrape_zips(author)
					if err != nil {
						p.Send(logMsg{Content: output.String(fmt.Sprintf("Error scraping zips from %s: %v", author, err)).Foreground(output.Color("1"))})
						mu.Lock()
						errors_ocurred = append(errors_ocurred, err.Error())
						mu.Unlock()
						continue
					}
					lang_zips = append(lang_zips, zips...)
				} else {
					p.Send(logMsg{Content: output.String(fmt.Sprintf("Skipping link %s. It does not match language %s\n", author, get_lang(i))).Bold().Underline()})
				}
			}

			unique := removeDuplicates(lang_zips)
			totalTasks := len(unique)
			p.Send(progressMsg{Index: i, Value: 0, Total: totalTasks})

			for _, talk := range unique {
				filename := filepath.Base(talk)
				path_to_file := filepath.Join(downloadDir, filename)

				if _, err := os.Stat(path_to_file); err == nil { // file exists
					p.Send(progressMsg{Index: i, Value: 1, Total: totalTasks})
					continue
				} else if errors.Is(err, os.ErrNotExist) {
					file, err := os.Create(path_to_file)
					if err != nil {
						p.Send(logMsg{Content: output.String(fmt.Sprintf("Terminating because the file %s could not be created: %v", path_to_file, err)).Foreground(output.Color("1"))})
						return
					}
					if err := download(talk, file); err != nil {
						p.Send(logMsg{Content: output.String(fmt.Sprintf("Error downloading %s: %v", talk, err)).Foreground(output.Color("1"))})
						mu.Lock()
						errors_ocurred = append(errors_ocurred, err.Error())
						mu.Unlock()
					} else {
						p.Send(logMsg{Content: output.String(fmt.Sprintf("Downloaded: %s", talk)).Foreground(output.Color("34"))})
						mu.Lock()
						new_downloads = append(new_downloads, filename)
						mu.Unlock()
					}
				} else {
					p.Send(logMsg{Content: output.String(fmt.Sprintf("Error downloading %s: %v", path_to_file, err)).Foreground(output.Color("1"))})
					mu.Lock()
					errors_ocurred = append(errors_ocurred, "Error downloading", talk, err.Error())
					mu.Unlock()
				}
				p.Send(progressMsg{Index: i, Value: 1, Total: totalTasks})
			}
		}(i, lang)
	}
	pages_wg.Wait()

	p.Send(logMsg{Content: output.String("All available files have been downloaded. New downloads include:").Bold().Underline().Foreground(output.Color("34"))})

	for _, name := range new_downloads {
		p.Send(logMsg{Content: output.String(name).Foreground(output.Color("34"))})
	}
	if len(new_downloads) == 0 {
		p.Send(logMsg{Content: output.String("None")})
	}

	if len(errors_ocurred) > 0 {
		p.Send(logMsg{Content: output.String("\nThe following errors occurred:").Bold().Underline().Foreground(output.Color("1"))})

		for _, e := range errors_ocurred {
			p.Send(logMsg{Content: output.String(e).Foreground(output.Color("1"))})
		}
	}
}
