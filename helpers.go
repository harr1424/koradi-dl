package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/net/html"
)

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

func removeDuplicates(input []string) []string {
	encountered := map[string]bool{}
	result := []string{}

	for _, value := range input {
		if !encountered[value] {
			encountered[value] = true
			result = append(result, value)
		}
	}

	return result
}

func scrape_authors(url string) ([]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch URL %s: %w", url, err)
	}
	defer resp.Body.Close()

	z := html.NewTokenizer(resp.Body)

	var links []string

	for {
		tt := z.Next()

		switch {
		case tt == html.ErrorToken:
			if z.Err() == io.EOF {
				return links, nil
			}
			return nil, z.Err()
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

func scrape_zips(url string) ([]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch URL %s: %w", url, err)
	}
	defer resp.Body.Close()

	z := html.NewTokenizer(resp.Body)
	var links []string

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if z.Err() == io.EOF {
				return links, nil
			}
			return nil, z.Err()
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			if t.Data == "a" {
				for _, a := range t.Attr {
					if a.Key == "href" {
						if strings.HasSuffix(a.Val, ".zip") || strings.HasSuffix(a.Val, "-zip") {
							links = append(links, a.Val)
						}
					}
				}
			}
		}
	}
}

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
