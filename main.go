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

	"github.com/muesli/termenv"
)

func main() {
	output := termenv.NewOutput(os.Stdout)
	msg := output.String("Welcome to the Koradi Archive Utility\n").
		Bold().
		Underline()
	fmt.Println(msg)

	//ensureElevatedPrivileges()
	confirmWorkingDir()

	run()

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

func run() {
	output := termenv.NewOutput(os.Stdout)
	var new_downloads []string
	var errors_ocurred []string
	var mu sync.Mutex

	// get client working directory and output so user knows where to locate downloaded files
	exPath, err := os.Executable()
	if err != nil {
		msg := output.String("Unable to detect working directory:").
			Bold().
			Underline().
			Foreground(output.Color("1"))
		fmt.Println(msg)

		return
	}
	exDir := filepath.Dir(exPath)
	if err := os.Chdir(exDir); err != nil {
		msg := output.String("Unable to change working directory:", err.Error()).
			Bold().
			Underline().
			Foreground(output.Color("1"))
		fmt.Println(msg)

		return
	}
	wd, err := os.Getwd()
	if err != nil {
		msg := output.String("Unable to detect working directory:", err.Error()).
			Bold().
			Underline().
			Foreground(output.Color("1"))
		fmt.Println(msg)

		return
	}
	fmt.Printf("Files will be downloaded to:  %s\n\n", wd)

	fmt.Printf("Searching for available downloads...\n\n")

	urls := [6]string{
		"https://koradi.org/en/downloads/",
		"https://koradi.org/es/descargas/",
		"https://koradi.org/fr/telechargements/",
		"https://koradi.org/po/downloads/",
		"https://koradi.org/it/download/",
		"https://koradi.org/de/herunterladen/",
	}

	/*
		Map containing int key and string array value. The string array will contain
		html pages of each author for a specific language, identified by the key.

		The helper function get_lang() will be used to convert the int key into a
		language abbreviation when iterating over the map.

		Keys must be integers for compatibility with the range based for loop used to
		iterate over this collection.
	*/
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
				msg := output.String(fmt.Sprintf("Error scraping authors from %s: %v", v, err)).
					Foreground(output.Color("1"))
				fmt.Println(msg)
				mu.Lock()
				errors_ocurred = append(errors_ocurred, err.Error())
				mu.Unlock()
				return
			}
			mu.Lock()
			pages[i] = links
			mu.Unlock()
		}(i, v)
	}
	lang_wg.Wait()

	var pages_wg sync.WaitGroup
	pages_wg.Add(len(pages))
	for i, lang := range pages { // for each language, get .zip downloads from author links
		go func(i int, lang []string) {
			defer pages_wg.Done()

			var lang_zips []string

			log.Printf("Checking %v %v links for .zip files...\n", len(pages[i]), get_lang(i))

			downloadDir := get_lang(i) // create a local dir for current language
			if err := os.MkdirAll(downloadDir, 0755); err != nil {
				msg := output.String("Terminating because a directory could not be created:", err.Error()).
					Foreground(output.Color("1"))
				fmt.Println(msg)

				return
			}

			for _, author := range lang {
				if strings.Contains(author, "/"+get_lang(i)+"/") {
					log.Println("Found", author)
					zips, err := scrape_zips(author)
					if err != nil {
						msg := output.String(fmt.Sprintf("Error scraping zips from %s: %v", author, err)).
							Foreground(output.Color("1"))
						fmt.Println(msg)
						mu.Lock()
						errors_ocurred = append(errors_ocurred, err.Error())
						mu.Unlock()
						continue
					}
					lang_zips = append(lang_zips, zips...)
				} else {
					log.Printf("Skipping link %s. It does not match language %s", author, get_lang(i))
				}
			}

			unique := removeDuplicates(lang_zips)
			totalFiles := len(unique)

			for j, talk := range unique {
				filename := filepath.Base(talk)
				path_to_file := filepath.Join(downloadDir, filename)

				if _, err := os.Stat(path_to_file); err == nil { // file exits
					fmt.Printf("%s %d/%d: File %s has been downloaded previously.\n", get_lang(i), j+1, totalFiles, talk)
					continue
				} else if errors.Is(err, os.ErrNotExist) { // file does not exist

					err := os.MkdirAll(filepath.Dir(path_to_file), 0755) // create dirdctory to hold file
					if err != nil {
						msg := output.String(fmt.Sprintf("Terminating because the directory %s could not be created: %v", path_to_file, err)).
							Foreground(output.Color("1"))
						fmt.Println(msg)

						return
					}
					file, err := os.Create(path_to_file) // create file to download to
					if err != nil {
						msg := output.String(fmt.Sprintf("Terminating because the file %s could not be created: %v", path_to_file, err.Error())).
							Foreground(output.Color("1"))
						fmt.Println(msg)

						return
					}
					if err := download(talk, file); err != nil { // donwload had errors
						msg := output.String(fmt.Sprintf("%s %d/%d: Error downloading %s %v", get_lang(i), j+1, totalFiles, path_to_file, err.Error())).
							Foreground(output.Color("1"))
						fmt.Println(msg)
						mu.Lock()
						errors_ocurred = append(errors_ocurred, err.Error())
						mu.Unlock()

					} else { // download succeeded
						msg := output.String(fmt.Sprintf("%s %d/%d: Downloaded: %s", get_lang(i), j+1, totalFiles, talk)).
							Foreground(output.Color("34"))
						fmt.Println(msg)
						mu.Lock()
						new_downloads = append(new_downloads, filename)
						mu.Unlock()
					}
				} else { // file does not exist and some other error ocurred
					msg := output.String(fmt.Sprintf("%s %d/%d: Error downloading %s %v", get_lang(i), j+1, totalFiles, path_to_file, err.Error())).
						Foreground(output.Color("1"))
					fmt.Println(msg)
					mu.Lock()
					errors_ocurred = append(errors_ocurred, "Error downloading", talk, err.Error())
					mu.Unlock()
				}

			}

		}(i, lang)
	}
	pages_wg.Wait()

	msg := output.String("All available files have been downloaded. New downloads include:").
		Bold().
		Underline()
	fmt.Println(msg)

	for _, name := range new_downloads {
		msg := output.String(name)
		fmt.Println(msg)
	}
	if len(new_downloads) == 0 {
		fmt.Println("None")
	}

	if len(errors_ocurred) > 0 {
		msg = output.String("\nThe following errors ocurred:").
			Bold().
			Underline().
			Foreground(output.Color("1"))
		fmt.Println(msg)

		for _, e := range errors_ocurred {
			msg := output.String(e).
				Foreground(output.Color("1"))
			fmt.Println(msg)
		}
	}
}
