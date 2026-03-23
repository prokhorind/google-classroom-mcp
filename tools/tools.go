package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prokhorind/google-classroom-mcp/classroom"
	googleclassroom "google.golang.org/api/classroom/v1"
)

type GetSubmissionsInput struct {
	ClassName string `json:"class_name" jsonschema:"Human-readable course name, partial match supported"`
	TaskName  string `json:"task_name"  jsonschema:"Human-readable assignment name, partial match supported"`
	OutputDir string `json:"output_dir" jsonschema:"Base directory to save files, defaults to ./submissions"`
}

type SaveGradeInput struct {
	CourseID        string  `json:"course_id"        jsonschema:"The course ID returned by get_submissions"`
	AssignmentTitle string  `json:"assignment_title" jsonschema:"The assignment title returned by get_submissions"`
	StudentID       string  `json:"student_id"       jsonschema:"The student ID returned by get_submissions"`
	Grade           float64 `json:"grade"            jsonschema:"Numeric grade to assign"`
	MaxGrade        float64 `json:"max_grade"        jsonschema:"Maximum possible grade"`
	AnswerFile      string  `json:"answer_file"      jsonschema:"Path to the teacher reference answer file used for grading"`
	Feedback        string  `json:"feedback"         jsonschema:"Optional grading notes or feedback for the student"`
}

type GradeEntry struct {
	StudentID  string    `json:"student_id"`
	Grade      float64   `json:"grade"`
	MaxGrade   float64   `json:"max_grade"`
	AnswerFile string    `json:"answer_file"`
	Feedback   string    `json:"feedback,omitempty"`
	GradedAt   time.Time `json:"graded_at"`
}

// Register adds all tools to the MCP server.
func Register(server *mcp.Server, svc *googleclassroom.Service, httpClient *http.Client, submissionsDir string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_submissions",
		Description: "Resolve a course and assignment by name, download all student submissions locally, and return their file paths for grading.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in GetSubmissionsInput) (*mcp.CallToolResult, any, error) {
		outDir := in.OutputDir
		if outDir == "" {
			outDir = submissionsDir
		}

		course, err := findCourse(ctx, svc, in.ClassName)
		if err != nil {
			return nil, nil, err
		}

		assignment, err := findAssignment(ctx, svc, course.ID, in.TaskName)
		if err != nil {
			return nil, nil, err
		}

		submissions, err := classroom.DownloadSubmissions(ctx, svc, httpClient, course.ID, assignment.ID, assignment.Title, outDir)
		if err != nil {
			return nil, nil, err
		}

		return textResult(map[string]any{
			"course_id":        course.ID,
			"course_name":      course.Name,
			"assignment_id":    assignment.ID,
			"assignment_title": assignment.Title,
			"max_grade":        assignment.MaxPoints,
			"output_dir":       outDir,
			"submissions":      submissions,
		})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "save_grade",
		Description: "Save a grade and feedback for a student locally to <output_dir>/<course_id>/<assignment_title>/grades.json. Reads the answer file to record the reference used for grading.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SaveGradeInput) (*mcp.CallToolResult, any, error) {
		if in.AnswerFile == "" {
			return nil, nil, fmt.Errorf("answer_file is required")
		}
		if _, err := os.Stat(in.AnswerFile); err != nil {
			return nil, nil, fmt.Errorf("answer_file not found: %s", in.AnswerFile)
		}
		if err := saveGrade(in, submissionsDir); err != nil {
			return nil, nil, err
		}
		return textResult(fmt.Sprintf("Grade %.1f/%.1f saved for student %s", in.Grade, in.MaxGrade, in.StudentID))
	})
}

// findCourse finds a course by case-insensitive partial name match.
func findCourse(ctx context.Context, svc *googleclassroom.Service, name string) (classroom.Course, error) {
	courses, err := classroom.ListCourses(ctx, svc)
	if err != nil {
		return classroom.Course{}, err
	}
	lower := strings.ToLower(name)
	for _, c := range courses {
		if strings.Contains(strings.ToLower(c.Name), lower) {
			return c, nil
		}
	}
	return classroom.Course{}, fmt.Errorf("no course matching %q", name)
}

// findAssignment finds an assignment by case-insensitive partial name match.
func findAssignment(ctx context.Context, svc *googleclassroom.Service, courseID, name string) (classroom.Assignment, error) {
	assignments, err := classroom.ListAssignments(ctx, svc, courseID)
	if err != nil {
		return classroom.Assignment{}, err
	}
	lower := strings.ToLower(name)
	for _, a := range assignments {
		if strings.Contains(strings.ToLower(a.Title), lower) {
			return a, nil
		}
	}
	return classroom.Assignment{}, fmt.Errorf("no assignment matching %q", name)
}

func saveGrade(in SaveGradeInput, submissionsDir string) error {
	dir := filepath.Join(submissionsDir, in.CourseID, classroom.Sanitize(in.AssignmentTitle))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	gradesFile := filepath.Join(dir, "grades.json")

	var grades []GradeEntry
	if data, err := os.ReadFile(gradesFile); err == nil {
		_ = json.Unmarshal(data, &grades)
	}

	entry := GradeEntry{
		StudentID:  in.StudentID,
		Grade:      in.Grade,
		MaxGrade:   in.MaxGrade,
		AnswerFile: in.AnswerFile,
		Feedback:   in.Feedback,
		GradedAt:   time.Now().UTC(),
	}
	updated := false
	for i, g := range grades {
		if g.StudentID == in.StudentID {
			grades[i] = entry
			updated = true
			break
		}
	}
	if !updated {
		grades = append(grades, entry)
	}

	data, err := json.MarshalIndent(grades, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(gradesFile, data, 0644)
}

func textResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, nil, nil
}
