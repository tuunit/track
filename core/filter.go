package core

import "time"

// FilterFunction is an alias for func(r *Record) bool
type FilterFunction = func(r *Record) bool

// FilterFunctions is an alias for []func(r *Record) bool
type FilterFunctions = []FilterFunction

// Filter checks a record using multiple filters
func Filter(record *Record, filters FilterFunctions) bool {
	for _, f := range filters {
		if !f(record) {
			return false
		}
	}
	return true
}

// FilterByProjects returns a function for filtering by project names
func FilterByProjects(projects []string) FilterFunction {
	prj := make(map[string]bool)
	for _, p := range projects {
		prj[p] = true
	}
	return func(r *Record) bool {
		_, ok := prj[r.Project]
		return ok
	}
}

// FilterByTime returns a function for filtering by time
//
// Keeps all records that are partially included in the given time span.
// Zero times in the given time span are ignored, resulting in an open time span.
//
// For records with a zero end, only the start time is compared
func FilterByTime(start, end time.Time) FilterFunction {
	return func(r *Record) bool {
		if r.End.IsZero() {
			return (start.IsZero() || r.Start.After(start)) && (end.IsZero() || r.Start.Before(end))
		}
		return (start.IsZero() || r.End.After(start)) && (end.IsZero() || r.Start.Before(end))
	}
}

// FilterByArchived returns a function for filtering by archived/not archived
func FilterByArchived(archived bool, projects map[string]Project) FilterFunction {
	return func(r *Record) bool {
		return projects[r.Project].Archived == archived
	}
}

// FilterByTagsAny returns a function for filtering by tags
func FilterByTagsAny(tags []string) FilterFunction {
	tg := make(map[string]bool)
	for _, t := range tags {
		tg[t] = true
	}
	return func(r *Record) bool {
		for _, t := range r.Tags {
			if _, ok := tg[t]; ok {
				return true
			}
		}
		return false
	}
}

// FilterByTagsAll returns a function for filtering by tags
func FilterByTagsAll(tags []string) FilterFunction {
	return func(r *Record) bool {
		for _, t := range tags {
			found := false
			for _, t2 := range r.Tags {
				if t == t2 {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
}
