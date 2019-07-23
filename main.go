package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/loganstone/gdpull/client"
	"golang.org/x/net/context"
	drive "google.golang.org/api/drive/v3"
)

const maxDownloadAtOnce = 5

var regexFilter *regexp.Regexp
var srv *drive.Service
var foundFiles map[string]string
var downloadQueue chan bool

func init() {
	if len(os.Args) < 2 {
		log.Fatal("enter target file\n")
	}
	foundFiles = make(map[string]string)
	downloadQueue = make(chan bool, maxDownloadAtOnce)
}

func download(srv *drive.Service, id, name string) error {
	log.Printf("Download - %s\n", name)
	resp, err := srv.Files.Get(id).Download()
	if err != nil {
		log.Println(err)
		log.Printf("Download failed - %s\n", name)
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(name)
	if err != nil {
		log.Printf("os.Create failed - %s\n", name)
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Printf("io.Copy failed - %s\n", name)
		return err
	}
	return nil
}

func procPage(r *drive.FileList) error {
	if len(r.Files) == 0 {
		return errors.New("no files found")
	}

	for _, f := range r.Files {
		if regexFilter.MatchString(f.Name) {
			foundFiles[f.Id] = f.Name
		}
	}

	return nil
}

func showFoundFiles() {
	fmt.Printf("Found files (%d):\n", len(foundFiles))
	num := 1
	for _, name := range foundFiles {
		fmt.Printf("%d. %s\n", num, name)
		num++
	}
}

func shouldDownload() bool {
	var response string
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Do you want to download it? (y/n): ")
	response, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	response = strings.Trim(response, " \n")
	if response != "y" && response != "n" {
		return shouldDownload()
	}
	if response == "n" {
		return false
	}
	return true
}

func main() {
	client, err := client.New()
	if err != nil {
		log.Fatal(err)
	}

	srv, err := drive.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve Drive client: %v", err)
	}

	regexFilter, err = regexp.Compile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	err = srv.Files.List().Pages(ctx, procPage)
	if err != nil {
		log.Fatal(err)
	}

	if len(foundFiles) == 0 {
		fmt.Println("No such files")
		return
	}

	showFoundFiles()

	if !shouldDownload() {
		os.Exit(0)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	for id, name := range foundFiles {
		downloadQueue <- true
		go func(id, name string) {
			download(srv, id, name)
			<-downloadQueue
			if len(downloadQueue) == 0 {
				wg.Done()
			}
		}(id, name)
	}
	wg.Wait()
}
