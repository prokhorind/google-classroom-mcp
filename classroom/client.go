package classroom

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googleclassroom "google.golang.org/api/classroom/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

var scopes = []string{
	googleclassroom.ClassroomCoursesReadonlyScope,
	googleclassroom.ClassroomCourseworkStudentsScope,
	googleclassroom.ClassroomCourseworkStudentsReadonlyScope,
	googleclassroom.ClassroomStudentSubmissionsStudentsReadonlyScope,
	googleclassroom.ClassroomCourseworkMeScope,
	googleclassroom.ClassroomRostersReadonlyScope,
	googleclassroom.ClassroomProfileEmailsScope,
	googleclassroom.ClassroomProfilePhotosScope,
	drive.DriveReadonlyScope,
}

func oauthConfig(clientID, clientSecret string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       scopes,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
	}
}

// NewService creates an authenticated Google Classroom service.
// Fails immediately if no cached token exists — run `go run ./cmd/auth` first.
func NewService(ctx context.Context, clientID, clientSecret, tokenFile string) (*googleclassroom.Service, *http.Client, error) {
	config := oauthConfig(clientID, clientSecret)

	tok, err := loadToken(tokenFile)
	if err != nil {
		return nil, nil, fmt.Errorf("no token found at %s — run `go run ./cmd/auth` first: %w", tokenFile, err)
	}

	httpClient := config.Client(ctx, tok)
	svc, err := googleclassroom.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, nil, fmt.Errorf("creating classroom service: %w", err)
	}
	return svc, httpClient, nil
}

// RunAuthFlow performs the OAuth2 browser flow and saves the token to tokenFile.
func RunAuthFlow(ctx context.Context, clientID, clientSecret, tokenFile string) error {
	config := oauthConfig(clientID, clientSecret)

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("\nOpen this URL in your browser:\n\n%s\n\nPaste the authorization code: ", authURL)

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return fmt.Errorf("reading auth code: %w", err)
	}

	tok, err := config.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("exchanging auth code: %w", err)
	}

	if err := saveToken(tokenFile, tok); err != nil {
		return fmt.Errorf("saving token: %w", err)
	}

	fmt.Printf("\nToken saved to %s — you can now run the MCP server.\n", tokenFile)
	return nil
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
