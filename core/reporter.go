package core

import (
	"fmt"
	"time"

	"golang.org/x/exp/maps"
)

// TimeRange represents a time range
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// Duration calculates the duration of a time range
func (r TimeRange) Duration() time.Duration {
	return r.End.Sub(r.Start)
}

// Reporter for generating reports
type Reporter struct {
	Track        *Track
	Records      []Record
	Projects     map[string]Project
	ProjectTime  map[string]time.Duration
	AllProjects  map[string]Project
	ProjectsTree *ProjectTree
	TimeRange    TimeRange
}

// NewReporter creates a new Reporter from filters
func NewReporter(t *Track, proj []string, filters FilterFunctions) (*Reporter, error) {
	allProjects, err := t.LoadAllProjects()
	if err != nil {
		return nil, err
	}
	projectsTree := ToProjectTree(allProjects)

	projects := make(map[string]Project)
	if len(proj) == 0 {
		projects = allProjects
	} else {
		for _, p := range proj {
			project := allProjects[p]
			projects[project.Name] = project

			desc, ok := projectsTree.Descendants(project.Name)
			if !ok {
				return nil, fmt.Errorf("BUG! Project '%s' not in project tree", project.Name)
			}
			for _, p2 := range desc {
				if _, ok = projects[p2.Value.Name]; !ok {
					projects[p2.Value.Name] = p2.Value
				}
			}
		}
	}

	filters = append(filters, FilterByProjects(maps.Keys(projects)))
	records, err := t.LoadAllRecordsFiltered(filters)
	if err != nil {
		return nil, err
	}

	totals := make(map[string]time.Duration, len(projects))
	for _, p := range projects {
		totals[p.Name] = time.Second * 0.0
	}

	tRange := TimeRange{}
	for _, rec := range records {
		totals[rec.Project] = totals[rec.Project] + rec.Duration()
		if tRange.Start.IsZero() || rec.Start.Before(tRange.Start) {
			tRange.Start = rec.Start
		}
		if rec.End.IsZero() {
			if tRange.End.IsZero() || rec.Start.After(tRange.End) {
				tRange.End = rec.Start
			}
		} else {
			if tRange.End.IsZero() || rec.End.After(tRange.End) {
				tRange.End = rec.End
			}
		}
	}

	for project := range totals {
		anc, ok := projectsTree.Ancestors(project)
		if !ok {
			return nil, fmt.Errorf("BUG! Project '%s' not in project tree", project)
		}
		for _, node := range anc {
			if _, ok := totals[node.Value.Name]; ok {
				totals[node.Value.Name] += totals[project]
			}
		}
	}

	report := Reporter{
		Track:        t,
		Records:      records,
		Projects:     projects,
		ProjectTime:  totals,
		AllProjects:  allProjects,
		ProjectsTree: projectsTree,
		TimeRange:    tRange,
	}
	return &report, nil
}
