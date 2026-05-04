package classroom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	googleclassroom "google.golang.org/api/classroom/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const (
	retryMaxAttempts = 3
	retryBaseDelay   = 2 * time.Second
)

// isRetryable returns true for Google API 5xx server errors that are worth retrying.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *googleapi.Error
	if ok := errorAs(err, &apiErr); ok {
		return apiErr.Code >= 500 && apiErr.Code < 600
	}
	return false
}

// errorAs is a thin wrapper so we can keep the import of "errors" out of the main file.
func errorAs(err error, target interface{}) bool {
	type asInterface interface {
		As(interface{}) bool
	}
	// Use the standard errors.As behaviour via the googleapi.Error pointer check.
	var apiErr *googleapi.Error
	switch t := target.(type) {
	case **googleapi.Error:
		for err != nil {
			if e, ok := err.(*googleapi.Error); ok {
				*t = e
				_ = apiErr
				return true
			}
			type unwrapper interface{ Unwrap() error }
			if u, ok := err.(unwrapper); ok {
				err = u.Unwrap()
			} else {
				break
			}
		}
	}
	return false
}

// withRetry calls fn up to retryMaxAttempts times, backing off on 5xx errors.
func withRetry(ctx context.Context, label string, fn func() error) error {
	var err error
	for attempt := 1; attempt <= retryMaxAttempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !isRetryable(err) {
			return err
		}
		delay := retryBaseDelay * time.Duration(attempt)
		log.Printf("[retry] %s: attempt %d/%d failed with %v — retrying in %s", label, attempt, retryMaxAttempts, err, delay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return fmt.Errorf("%s failed after %d attempts: %w", label, retryMaxAttempts, err)
}

type Submission struct {
	StudentID    string           `json:"student_id"`
	StudentName  string           `json:"student_name"`
	StudentEmail string           `json:"student_email"`
	Version      string           `json:"version"`
	Files        []DownloadedFile `json:"files"`
}

type DownloadedFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Google Docs MIME types — exported as plain text
var googleDocsMimeTypes = map[string]struct{}{
	"application/vnd.google-apps.document":     {},
	"application/vnd.google-apps.spreadsheet":  {},
	"application/vnd.google-apps.presentation": {},
}

// DownloadSubmissions fetches all student submissions for an assignment and
// saves them under baseDir/<courseID>/<assignmentTitle>/<studentID>/<timestamp>/
// Each download creates a new timestamped version. Always returns the latest version paths.
func DownloadSubmissions(ctx context.Context, svc *googleclassroom.Service, httpClient *http.Client, courseID, courseWorkID, assignmentTitle, baseDir string) ([]Submission, error) {
	driveSvc, err := drive.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("creating drive service: %w", err)
	}

	timestamp := time.Now().UTC().Format("2006-01-02T15-04-05")
	var submissions []Submission

	log.Printf("[get_submissions] starting download: course=%s assignment=%q dir=%s", courseID, assignmentTitle, baseDir)

	err = svc.Courses.CourseWork.StudentSubmissions.
		List(courseID, courseWorkID).
		Pages(ctx, func(page *googleclassroom.ListStudentSubmissionsResponse) error {
			for _, sub := range page.StudentSubmissions {
				log.Printf("[get_submissions] processing student=%s", sub.UserId)

				// Each download is a new timestamped version
				versionDir := filepath.Join(baseDir, courseID, Sanitize(assignmentTitle), sub.UserId, timestamp)
				if err := os.MkdirAll(versionDir, 0755); err != nil {
					return err
				}

				var downloaded []DownloadedFile

				// Attachments from the assignment submission
				if sub.AssignmentSubmission != nil {
					for _, att := range sub.AssignmentSubmission.Attachments {
						var df *DownloadedFile
						var err error
						switch {
						case att.DriveFile != nil:
							log.Printf("[get_submissions]   drive file: id=%s title=%q", att.DriveFile.Id, att.DriveFile.Title)
							df, err = handleDriveAttachment(ctx, driveSvc, att.DriveFile, versionDir)
						case att.Link != nil:
							log.Printf("[get_submissions]   link: %s", att.Link.Url)
							df, err = handleLinkAttachment(att.Link, versionDir)
						}
						if err != nil {
							// Log and skip files that fail to download (e.g. Google 500 on export).
							// A failed file should not abort the entire submission batch.
							log.Printf("[get_submissions]   ERROR: %v", err)
							skipped := saveSkippedFile(att, err, versionDir)
							if skipped != nil {
								downloaded = append(downloaded, *skipped)
							}
							continue
						}
						if df != nil {
							log.Printf("[get_submissions]   saved: %s", df.Path)
							downloaded = append(downloaded, *df)
						}
					}
				}

				// Short answer text submitted by the student
				if sub.ShortAnswerSubmission != nil && sub.ShortAnswerSubmission.Answer != "" {
					df, err := saveTextSubmission("short_answer.txt", sub.ShortAnswerSubmission.Answer, versionDir)
					if err != nil {
						return err
					}
					downloaded = append(downloaded, *df)
				}

				profile, err := GetStudentProfile(ctx, svc, courseID, sub.UserId)
				if err != nil {
					log.Printf("[get_submissions]   WARN: could not fetch profile for %s: %v", sub.UserId, err)
					profile = StudentProfile{ID: sub.UserId}
				}

				if err := writeStudentInfo(versionDir, profile); err != nil {
					return fmt.Errorf("writing student info: %w", err)
				}

				log.Printf("[get_submissions] done student=%s name=%q files=%d", sub.UserId, profile.FullName, len(downloaded))

				submissions = append(submissions, Submission{
					StudentID:    sub.UserId,
					StudentName:  profile.FullName,
					StudentEmail: profile.Email,
					Version:      timestamp,
					Files:        downloaded,
				})
			}
			return nil
		})

	log.Printf("[get_submissions] finished: total=%d err=%v", len(submissions), err)
	return submissions, err
}

// handleDriveAttachment downloads any Drive file, or exports Google Docs as plain text.
func handleDriveAttachment(ctx context.Context, svc *drive.Service, df *googleclassroom.DriveFile, destDir string) (*DownloadedFile, error) {
	var meta *drive.File
	err := withRetry(ctx, fmt.Sprintf("fetch metadata %s", df.Title), func() error {
		var e error
		meta, e = svc.Files.Get(df.Id).Fields("mimeType", "name").Context(ctx).Do()
		return e
	})
	if err != nil {
		return nil, fmt.Errorf("fetching metadata for %s: %w", df.Title, err)
	}

	if _, isGoogleDoc := googleDocsMimeTypes[meta.MimeType]; isGoogleDoc {
		return exportGoogleDoc(ctx, svc, df, destDir, meta.MimeType)
	}

	// Download all regular files regardless of extension
	destPath := filepath.Join(destDir, df.Title)
	if err := downloadDriveFile(ctx, svc, df.Id, destPath); err != nil {
		return nil, fmt.Errorf("downloading %s: %w", df.Title, err)
	}
	return &DownloadedFile{Name: df.Title, Path: destPath}, nil
}

// exportGoogleDoc exports a Google Doc/Sheet/Slide as plain text.
func exportGoogleDoc(ctx context.Context, svc *drive.Service, df *googleclassroom.DriveFile, destDir string, mimeType string) (*DownloadedFile, error) {
	exportMime := "text/plain"
	if mimeType == "application/vnd.google-apps.spreadsheet" {
		exportMime = "text/csv"
	}

	name := df.Title + ".txt"
	destPath := filepath.Join(destDir, name)

	var resp *http.Response
	err := withRetry(ctx, fmt.Sprintf("export %s", df.Title), func() error {
		var e error
		resp, e = svc.Files.Export(df.Id, exportMime).Context(ctx).Download()
		return e
	})
	if err != nil {
		return nil, fmt.Errorf("exporting %q as plain text: %w", df.Title, err)
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

// handleLinkAttachment saves a link as a .txt file with the URL.
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

// saveTextSubmission writes a plain text string to a file in the student dir.
func saveTextSubmission(filename, content, destDir string) (*DownloadedFile, error) {
	destPath := filepath.Join(destDir, filename)
	if err := os.WriteFile(destPath, []byte(content), 0644); err != nil {
		return nil, err
	}
	return &DownloadedFile{Name: filename, Path: destPath}, nil
}

func writeStudentInfo(dir string, p StudentProfile) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "student.json"), data, 0644)
}

func Sanitize(s string) string {
	return strings.NewReplacer(" ", "_", "/", "-", "\\", "-").Replace(s)
}

func downloadDriveFile(ctx context.Context, svc *drive.Service, fileID, destPath string) error {
	var resp *http.Response
	err := withRetry(ctx, fmt.Sprintf("download file %s", fileID), func() error {
		var e error
		resp, e = svc.Files.Get(fileID).Context(ctx).Download()
		return e
	})
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

// saveSkippedFile writes a .skipped file noting why a file could not be downloaded.
// Returns a DownloadedFile entry so the caller knows something was recorded.
func saveSkippedFile(att *googleclassroom.Attachment, reason error, destDir string) *DownloadedFile {
	title := "unknown"
	if att.DriveFile != nil {
		title = att.DriveFile.Title
	} else if att.Link != nil {
		title = att.Link.Title
	}

	log.Printf("WARN: skipping attachment %q: %v", title, reason)

	name := Sanitize(title) + ".skipped"
	destPath := filepath.Join(destDir, name)
	content := fmt.Sprintf("File: %s\nError: %v\n", title, reason)
	if err := os.WriteFile(destPath, []byte(content), 0644); err != nil {
		log.Printf("WARN: could not write skipped marker for %q: %v", title, err)
		return nil
	}
	return &DownloadedFile{Name: name, Path: destPath}
}
