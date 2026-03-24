package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prokhorind/google-classroom-mcp/classroom"
	googleclassroom "google.golang.org/api/classroom/v1"
)

type GetSubmissionsInput struct {
	ClassName string `json:"class_name" jsonschema:"Human-readable course name, partial match supported"`
	TaskName  string `json:"task_name"  jsonschema:"Human-readable assignment name, partial match supported"`
	OutputDir string `json:"output_dir" jsonschema:"Base directory to save files, defaults to the connected workspace root"`
}

// Register adds all tools to the MCP server.
func Register(server *mcp.Server, svc *googleclassroom.Service, httpClient *http.Client, submissionsDir string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_submissions",
		Description: "Resolve a course and assignment by name, download all student submissions locally, and return their file paths for grading.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in GetSubmissionsInput) (*mcp.CallToolResult, any, error) {
		outDir := in.OutputDir
		if outDir == "" {
			outDir = resolveOutputDir(ctx, req, submissionsDir)
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

// resolveOutputDir returns the best available output directory.
// Priority: first workspace root from client → SUBMISSIONS_DIR env fallback.
func resolveOutputDir(ctx context.Context, req *mcp.CallToolRequest, fallback string) string {
	if req != nil && req.Session != nil {
		result, err := req.Session.ListRoots(ctx, &mcp.ListRootsParams{})
		if err == nil && result != nil && len(result.Roots) > 0 {
			uri := result.Roots[0].URI
			path := strings.TrimPrefix(uri, "file://")
			if path != "" {
				return filepath.Join(path, "submissions")
			}
		}
	}
	return fallback
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
