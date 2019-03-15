package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	drive "google.golang.org/api/drive/v3"
)

const maxGoroutineForDownload = 5

var re *regexp.Regexp
var srv *drive.Service
var filteredFilesGroup []map[string]string
var totalMatchesCount int

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func init() {
	if len(os.Args) < 2 {
		log.Fatal("enter target file\n")
	}
	filteredFilesGroup = make([]map[string]string, 5)
	totalMatchesCount = 0
}

func download(srv *drive.Service, id string, name string) error {
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

	filteredFiles := make(map[string]string)
	idx := 0

	for _, i := range r.Files {
		if re.MatchString(i.Name) {
			filteredFiles[i.Id] = i.Name
			idx++
			totalMatchesCount++
			if idx == maxGoroutineForDownload {
				filteredFilesGroup = append(filteredFilesGroup, filteredFiles)
				filteredFiles = make(map[string]string)
				idx = 0
			}
		}
	}

	if len(filteredFiles) > 0 {
		filteredFilesGroup = append(filteredFilesGroup, filteredFiles)
	}
	return nil
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
	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, drive.DriveReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)

	srv, err := drive.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve Drive client: %v", err)
	}

	re, err = regexp.Compile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	err = srv.Files.List().Pages(ctx, procPage)
	if err != nil {
		log.Fatal(err)
	}
	if len(filteredFilesGroup) == 0 {
		fmt.Println("No such files")
		return
	}

	fmt.Printf("Found files (%d):\n", totalMatchesCount)
	num := 1
	for _, files := range filteredFilesGroup {
		for _, name := range files {
			fmt.Printf("%d. %s\n", num, name)
			num++
		}
	}

	if !shouldDownload() {
		os.Exit(0)
	}

	for _, files := range filteredFilesGroup {
		var wg sync.WaitGroup
		for id, name := range files {
			wg.Add(1)
			go func(id string, name string) {
				download(srv, id, name)
				wg.Done()
			}(id, name)
		}
		wg.Wait()
	}
}
