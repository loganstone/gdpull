package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"

	"golang.org/x/oauth2"
	drive "google.golang.org/api/drive/v3"
)

const (
	redirectURL = "urn:ietf:wg:oauth:2.0:oob"
	authURL     = "https://accounts.google.com/o/oauth2/auth"
	tokenURL    = "https://oauth2.googleapis.com/token"
)

// New is retrieve a token, saves the token, then returns the generated client and error.
func New() (*http.Client, error) {
	// The file .gdpull stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	config, err := getConfig()
	if err != nil {
		return nil, err
	}

	usr, err := user.Current()
	if err != nil {
		return nil, err
	}
	tokFilePath := filepath.Join(usr.HomeDir, ".gdpull")
	tok, err := tokenFromFile(tokFilePath)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFilePath, tok)
	}
	return config.Client(context.Background(), tok), nil
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	fmt.Print("Enter authorization code: ")

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

func getConfig() (*oauth2.Config, error) {
	const errMsgFmt = "'%s' environment variable is required"
	clientID, ok := os.LookupEnv("GDPULL_CLIENT_ID")
	if !ok {
		return nil, fmt.Errorf(errMsgFmt, "GDPULL_CLIENT_ID")
	}

	clientSecret, ok := os.LookupEnv("GDPULL_CLIENT_SECRET")
	if !ok {
		return nil, fmt.Errorf(errMsgFmt, "GDPULL_CLIENT_SECRET")
	}

	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{drive.DriveReadonlyScope},
		Endpoint: oauth2.Endpoint{
			AuthURL:  authURL,
			TokenURL: tokenURL,
		},
	}, nil
}
