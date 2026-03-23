package classroom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	googleclassroom "google.golang.org/api/classroom/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type Submission struct {
	StudentID    string           `json:"student_id"`
	StudentName  string           `json:"student_name"`
	StudentEmail string           `json:"student_email"`
	Files        []DownloadedFile `json:"files"`
}

type DownloadedFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Google Docs MIME types that we export as plain text
var googleDocsMimeTypes = map[string]struct{}{
	"application/vnd.google-apps.document":     {},
	"application/vnd.google-apps.spreadsheet":  {},
	"application/vnd.google-apps.presentation": {},
}

// DownloadSubmissions fetches all student submissions for an assignment and
// saves them under baseDir/<courseID>/<assignmentTitle>/<studentID>/
// Handles: .py/.sql Drive files, Google Docs/Sheets/Slides (exported as .txt), and link attachments.
func DownloadSubmissions(ctx context.Context, svc *googleclassroom.Service, httpClient *http.Client, courseID, courseWorkID, assignmentTitle, baseDir string) ([]Submission, error) {
	driveSvc, err := drive.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("creating drive service: %w", err)
	}

	var submissions []Submission

	err = svc.Courses.CourseWork.StudentSubmissions.
		List(courseID, courseWorkID).
		Pages(ctx, func(page *googleclassroom.ListStudentSubmissionsResponse) error {
			for _, sub := range page.StudentSubmissions {
				if sub.AssignmentSubmission == nil {
					continue
				}

				studentDir := filepath.Join(baseDir, courseID, Sanitize(assignmentTitle), sub.UserId)
				if err := os.MkdirAll(studentDir, 0755); err != nil {
					return err
				}

				var downloaded []DownloadedFile
				for _, att := range sub.AssignmentSubmission.Attachments {
					var df *DownloadedFile
					var err error

					switch {
					case att.DriveFile != nil:
						df, err = handleDriveAttachment(ctx, driveSvc, att.DriveFile, studentDir)
					case att.Link != nil:
						df, err = handleLinkAttachment(att.Link, studentDir)
					}

					if err != nil {
						return err
					}
					if df != nil {
						downloaded = append(downloaded, *df)
					}
				}

				profile, err := GetStudentProfile(ctx, svc, courseID, sub.UserId)
				if err != nil {
					// non-fatal — fall back to ID only
					profile = StudentProfile{ID: sub.UserId}
				}

				if err := writeStudentInfo(studentDir, profile); err != nil {
					return fmt.Errorf("writing student info: %w", err)
				}

				submissions = append(submissions, Submission{
					StudentID:    sub.UserId,
					StudentName:  profile.FullName,
					StudentEmail: profile.Email,
					Files:        downloaded,
				})
			}
			return nil
		})

	return submissions, err
}

// handleDriveAttachment downloads a Drive file or exports a Google Doc as plain text.
func handleDriveAttachment(ctx context.Context, svc *drive.Service, df *googleclassroom.DriveFile, destDir string) (*DownloadedFile, error) {
	// Fetch file metadata to get the MIME type
	meta, err := svc.Files.Get(df.Id).Fields("mimeType", "name").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("fetching metadata for %s: %w", df.Title, err)
	}

	if _, isGoogleDoc := googleDocsMimeTypes[meta.MimeType]; isGoogleDoc {
		return exportGoogleDoc(ctx, svc, df, destDir)
	}

	// Regular file — only download .py and .sql
	if !isRelevantFile(df.Title) {
		return nil, nil
	}
	destPath := filepath.Join(destDir, df.Title)
	if err := downloadDriveFile(ctx, svc, df.Id, destPath); err != nil {
		return nil, fmt.Errorf("downloading %s: %w", df.Title, err)
	}
	return &DownloadedFile{Name: df.Title, Path: destPath}, nil
}

// exportGoogleDoc exports a Google Doc/Sheet/Slide as plain text (.txt).
func exportGoogleDoc(ctx context.Context, svc *drive.Service, df *googleclassroom.DriveFile, destDir string) (*DownloadedFile, error) {
	name := df.Title + ".txt"
	destPath := filepath.Join(destDir, name)

	resp, err := svc.Files.Export(df.Id, "text/plain").Context(ctx).Download()
	if err != nil {
		return nil, fmt.Errorf("exporting google doc %s: %w", df.Title, err)
	}
	defer resp.Body.Close()

	f, err := os.Create(destPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return nil, err
	}
	return &DownloadedFile{Name: name, Path: destPath}, nil
}

// handleLinkAttachment saves a link submission as a .txt file containing the URL.
func handleLinkAttachment(link *googleclassroom.Link, destDir string) (*DownloadedFile, error) {
	title := link.Title
	if title == "" {
		title = "link"
	}
	name := Sanitize(title) + ".txt"
	destPath := filepath.Join(destDir, name)

	content := fmt.Sprintf("Title: %s\nURL: %s\n", link.Title, link.Url)
	if err := os.WriteFile(destPath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("saving link %s: %w", link.Url, err)
	}
	return &DownloadedFile{Name: name, Path: destPath}, nil
}

func writeStudentInfo(dir string, p StudentProfile) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "student.json"), data, 0644)
}

func isRelevantFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".sql")
}

func Sanitize(s string) string {
	return strings.NewReplacer(" ", "_", "/", "-", "\\", "-").Replace(s)
}

func downloadDriveFile(ctx context.Context, svc *drive.Service, fileID, destPath string) error {
	resp, err := svc.Files.Get(fileID).Context(ctx).Download()
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
