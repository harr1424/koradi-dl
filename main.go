/*
CREATED APRIL, 2023
ALL RIGHTS RESERVED TO THE KORADI GROUP

This program crawls the download page for each language on koradi.org searching for .zip files.
These files will be downloaded to the local directory where this program was executed.
Individual files will be grouped into directories specific to each language (en, es, fr, it, po, de).
*/

package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

var mu sync.Mutex

func main() {
	fmt.Printf("Welcome to the Koradi Archive Utility\n\n")

	// determine client platform and escalate privileges to create filesystem entries
	if runtime.GOOS == "windows" {
		out, err := exec.Command("net", "session").Output()
		if err != nil {
			log.Fatal("Unable to check for sufficient privileges. Terminating.")
		}

		if string(out) == "" {
			fmt.Println("This program requires elevated privileges in order to create directories and files on your computer. You will be prompted to enter your password.")
			cmd := exec.Command("runas", "/user:Administrator", os.Args[0])
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err := cmd.Run()
			if err != nil {
				log.Fatal("Unable to run as admin. Terminating.", err)
			}
		}
	} else {
		if os.Geteuid() != 0 {
			fmt.Println("This program requires elevated privileges in order to create directories and files on your computer. You will be prompted to enter your password:")
			cmd := exec.Command("sudo", os.Args[0])
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err := cmd.Run()
			if err != nil {
				log.Fatal("Unable to run as root. Terminating.", err)
			}
			os.Exit(0)
		}
	}

	run()
}

func run() {
	// get client working directory and output so user knows where to locate downloaded files
	exPath, err := os.Executable()
	if err != nil {
		fmt.Println("Unable to detect working directory:", err)
		return
	}
	exDir := filepath.Dir(exPath)
	if err := os.Chdir(exDir); err != nil {
		fmt.Println("Unable to change working directory:", err)
		return
	}
	wd, err := os.Getwd()
	if err != nil {
		fmt.Println("Unable to detect working directory:", err)
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

		Keys must be integers for compatability with the range based for loop used to
		iterate over this collection.
	*/
	var pages = map[int][]string{
		0: {},
		1: {},
		2: {},
		3: {},
		4: {},
		5: {},
	}

	/*
		A slice of files that have been downloaded during this invocation of the program.
		Used to notify the user which newly available files have been downloaded.
	*/
	var new_downloads []string

	/*
		For each page in pages (for each language), links to author pages will be scraped and
		stored in string arrays (value of pages).

		Subsequently these author pages will also be scraped for links pointing to .zip downloads.
		These links will be stored in lang_zips.

		A separate thread will be used to download .zip files belonging to each language.

		Link names will be used to create filesystem entries, where all individual talks belonging to a
		given language will be grouped in the same directory.

		Filesystem entries will be checked for existence, and existing files will not be downloaded again.
	*/
	var lang_wg sync.WaitGroup
	lang_wg.Add(len(urls))
	for i, v := range urls { // for each language get author links
		go func(i int, v string) {
			defer lang_wg.Done()
			pages[i] = scrape_authors(v)
		}(i, v)
	}
	lang_wg.Wait()

	for i, lang := range pages { // for each language, get .zip downloads from author links
		var lang_zips []string

		log.Printf("Checking %v %v links for .zip files...\n", len(pages[i]), get_lang(i))

		downloadDir := get_lang(i) // create a local dir for current language
		if err := os.MkdirAll(downloadDir, 0755); err != nil {
			log.Println(err)
			log.Fatal("Could not create directory as described above. Terminating.")
		}

		for _, author := range lang { // for each author, get .zip downloads

			// Check if the author URL contains the language code
			if strings.Contains(author, "/"+get_lang(i)+"/") {
				lang_zips = append(lang_zips, scrape_zips(author)...)
			} else {
				log.Printf("Skipping link %s. It does not match language %s", author, get_lang(i))
			}
		}

		for _, talk := range lang_zips {
			filename := filepath.Base(talk)
			path_to_file := filepath.Join(downloadDir, filename)

			// check if file already exists
			mu.Lock()
			if _, err := os.Stat(path_to_file); err == nil { // file exits
				mu.Unlock()
				continue
			} else if errors.Is(err, os.ErrNotExist) { // file does not exist
				file, err := os.Create(path_to_file)
				if err != nil {
					log.Println(err)
					log.Fatal("Could not create file as described above. Terminating.")
				}
				if err := download(talk, file); err != nil {
					log.Println(err)
				} else {
					log.Println("Downloaded", talk)
					new_downloads = append(new_downloads, filename)
				}
			} else {
				log.Println(err)
				log.Printf("File %s was not downloaded, see error above", path_to_file)
			}
			mu.Unlock()
		}

	}

	log.Println("All available files have been downloaded")

	fmt.Println("New downloads include: ")
	for _, name := range new_downloads {
		fmt.Println(name)
	}
	if len(new_downloads) == 0 {
		fmt.Println("None")
	}
}

/*
Helper function to get language being iterated over using range based for loop on map
*/
func get_lang(num int) string {
	switch num {
	case 0:
		return "en"
	case 1:
		return "es"
	case 2:
		return "fr"
	case 3:
		return "po"
	case 4:
		return "it"
	case 5:
		return "de"
	}

	return "invalid language"
}

/*
Given a URL, will return a string array containing all links found on the page.
These links are guaranteed to include authors, but may include other links as well.
*/
func scrape_authors(url string) []string {
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("WARNING: Files at url %s will not be downloaded due to the following error:\n", url)
		log.Println(err)
		return nil
	}
	defer resp.Body.Close()

	z := html.NewTokenizer(resp.Body)

	var links []string

	for {
		tt := z.Next()

		switch {
		case tt == html.ErrorToken:
			return links
		case tt == html.StartTagToken:
			t := z.Token()

			if t.Data == "a" {
				for _, a := range t.Attr {
					if a.Key == "href" && strings.HasSuffix(a.Val, "/") {
						links = append(links, a.Val)
					}
				}
			}
		}
	}
}

/*
Given a URL, will return a string array of all links that point to a .zip download.
*/
func scrape_zips(url string) []string {
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("WARNING: File at %s will not be downloaded due to the following error:\n", url)
		log.Println(err)
		return nil
	}
	defer resp.Body.Close()

	z := html.NewTokenizer(resp.Body)

	var links []string

	for {
		tt := z.Next()

		switch {
		case tt == html.ErrorToken:
			return links
		case tt == html.StartTagToken:
			t := z.Token()

			if t.Data == "a" {
				for _, a := range t.Attr {
					if a.Key == "href" && strings.HasSuffix(a.Val, ".zip") {
						links = append(links, a.Val)
					}
				}
			}
		}
	}
}

/*
Given a URL and local file object, copies the contents at URL to local file object.
*/
func download(url string, dest *os.File) error {
	defer dest.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(dest, resp.Body)

	return err
}
