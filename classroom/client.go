package classroom

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googleclassroom "google.golang.org/api/classroom/v1"
	"google.golang.org/api/option"
)

var scopes = []string{
	googleclassroom.ClassroomCoursesReadonlyScope,
	googleclassroom.ClassroomCourseworkStudentsScope,
	googleclassroom.ClassroomCourseworkStudentsReadonlyScope,
	googleclassroom.ClassroomStudentSubmissionsStudentsReadonlyScope,
	googleclassroom.ClassroomCourseworkMeScope,
	googleclassroom.ClassroomRostersReadonlyScope,
}

// NewService creates an authenticated Google Classroom service.
// credentialsFile is the path to credentials.json from Google Cloud Console.
// tokenFile is where the OAuth2 token is cached after the first auth flow.
// Returns the service and the underlying HTTP client (reused for Drive).
func NewService(ctx context.Context, credentialsFile, tokenFile string) (*googleclassroom.Service, *http.Client, error) {
	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", credentialsFile, err)
	}

	config, err := google.ConfigFromJSON(data, scopes...)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing credentials.json: %w", err)
	}

	tok, err := loadToken(tokenFile)
	if err != nil {
		tok, err = runAuthFlow(ctx, config, tokenFile)
		if err != nil {
			return nil, nil, fmt.Errorf("auth flow: %w", err)
		}
	}

	httpClient := config.Client(ctx, tok)
	svc, err := googleclassroom.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, nil, fmt.Errorf("creating classroom service: %w", err)
	}
	return svc, httpClient, nil
}

func loadToken(path string) (*oauth2.Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	return tok, json.NewDecoder(f).Decode(tok)
}

func saveToken(path string, tok *oauth2.Token) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("saving token to %s: %w", path, err)
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(tok)
}

// runAuthFlow prints an auth URL, waits for the user to paste the code back.
func runAuthFlow(ctx context.Context, config *oauth2.Config, tokenFile string) (*oauth2.Token, error) {
	config.RedirectURL = "urn:ietf:wg:oauth:2.0:oob"

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("\nOpen this URL in your browser:\n\n%s\n\nPaste the authorization code: ", authURL)

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return nil, fmt.Errorf("reading auth code: %w", err)
	}

	tok, err := config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchanging auth code: %w", err)
	}

	if err := saveToken(tokenFile, tok); err != nil {
		log.Printf("warning: could not cache token: %v", err)
	}

	return tok, nil
}
