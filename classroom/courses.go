package classroom

import (
	"context"
	"fmt"

	googleclassroom "google.golang.org/api/classroom/v1"
)

type Course struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Assignment struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	MaxPoints   float64 `json:"max_points"`
}

// ListCourses returns all active courses for the authenticated user.
func ListCourses(ctx context.Context, svc *googleclassroom.Service) ([]Course, error) {
	var courses []Course
	err := svc.Courses.List().CourseStates("ACTIVE").Pages(ctx, func(page *googleclassroom.ListCoursesResponse) error {
		for _, c := range page.Courses {
			courses = append(courses, Course{ID: c.Id, Name: c.Name})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing courses: %w", err)
	}
	return courses, nil
}

// ListAssignments returns all coursework items for a given course.
func ListAssignments(ctx context.Context, svc *googleclassroom.Service, courseID string) ([]Assignment, error) {
	var assignments []Assignment
	err := svc.Courses.CourseWork.List(courseID).Pages(ctx, func(page *googleclassroom.ListCourseWorkResponse) error {
		for _, cw := range page.CourseWork {
			assignments = append(assignments, Assignment{
				ID:          cw.Id,
				Title:       cw.Title,
				Description: cw.Description,
				MaxPoints:   cw.MaxPoints,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing assignments for course %s: %w", courseID, err)
	}
	return assignments, nil
}
