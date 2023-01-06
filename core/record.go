package core

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mlange-42/track/fs"
	"github.com/mlange-42/track/util"
)

// TagPrefix denotes tags in record notes
const TagPrefix = "+"

// CommentPrefix denotes comments in record files
const CommentPrefix = "#"

// YamlCommentPrefix denotes comments in YAML files
const YamlCommentPrefix = "#"

var (
	// ErrNoRecords is an error for no records found for a date
	ErrNoRecords = errors.New("no records for date")
	// ErrRecordNotFound is an error for a particular record not found
	ErrRecordNotFound = errors.New("record not found")
)

// Record holds and manipulates data for a record
type Record struct {
	Project string    `json:"project"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Note    string    `json:"note"`
	Tags    []string  `json:"tags"`
	Pause   []Pause   `json:"pause"`
}

// Pause holds information about a pause in a record
type Pause struct {
	Start time.Time
	End   time.Time
	Note  string
}

// FilterResult contains a Report or an error from async filtering
type FilterResult struct {
	Record Record
	Err    error
}

// Duration reports the duration of a pause
func (p *Pause) Duration(min, max time.Time) time.Duration {
	return util.DurationClip(p.Start, p.End, min, max)
}

// HasEnded reports whether the record has an end time
func (r *Record) HasEnded() bool {
	return !r.End.IsZero()
}

// IsPaused reports whether the record is paused
func (r *Record) IsPaused() bool {
	return len(r.Pause) > 0 && r.Pause[len(r.Pause)-1].End.IsZero()
}

// CurrentPause returns the current pause
func (r *Record) CurrentPause() (Pause, bool) {
	if len(r.Pause) > 0 && r.Pause[len(r.Pause)-1].End.IsZero() {
		return r.Pause[len(r.Pause)-1], true
	}
	return Pause{}, false
}

// LastPause returns the last pause. Returns false if there is an open pause
func (r *Record) LastPause() (Pause, bool) {
	if len(r.Pause) > 0 && !r.Pause[len(r.Pause)-1].End.IsZero() {
		return r.Pause[len(r.Pause)-1], true
	}
	return Pause{}, false
}

// Duration reports the duration of a record, excluding pause time
func (r *Record) Duration(min, max time.Time) time.Duration {
	dur := util.DurationClip(r.Start, r.End, min, max)
	dur -= r.PauseDuration(min, max)
	return dur
}

// TotalDuration reports the duration of a record, including pause time
func (r *Record) TotalDuration(min, max time.Time) time.Duration {
	return util.DurationClip(r.Start, r.End, min, max)
}

// PauseDuration reports the duration of all pauses of the record
func (r *Record) PauseDuration(min, max time.Time) time.Duration {
	dur := time.Second * 0
	for _, p := range r.Pause {
		dur += p.Duration(min, max)
	}
	return dur
}

// CurrentPauseDuration reports the duration of an open pause
func (r *Record) CurrentPauseDuration(min, max time.Time) time.Duration {
	dur := time.Second * 0
	if len(r.Pause) == 0 {
		return dur
	}
	if !r.IsPaused() {
		return dur
	}
	last := r.Pause[len(r.Pause)-1]

	return last.Duration(min, max)
}

// Check checks consistency of a record
func (r *Record) Check() error {
	if !r.End.IsZero() && r.End.Before(r.Start) {
		return fmt.Errorf("end time is before start time")
	}
	prevStart := util.NoTime
	prevEnd := util.NoTime
	for _, p := range r.Pause {
		if p.Start.Before(r.Start) {
			return fmt.Errorf("pause starts before record")
		}
		if !r.End.IsZero() && p.End.After(r.End) {
			return fmt.Errorf("pause ends after record")
		}
		if prevStart.After(p.Start) {
			return fmt.Errorf("pause starts not in chronological order")
		}
		if prevEnd.After(p.Start) {
			return fmt.Errorf("pauses overlap")
		}
		prevStart = p.Start
		prevEnd = p.End
	}
	return nil
}

// InsertPause inserts a pause into a record
func (r *Record) InsertPause(start time.Time, end time.Time, note string) (Pause, error) {
	if len(r.Pause) == 0 {
		if start.Before(r.Start) {
			return Pause{}, fmt.Errorf("start of pause before start of current record")
		}
	} else {
		if start.Before(r.Pause[len(r.Pause)-1].End) {
			return Pause{}, fmt.Errorf("start of pause before end of previous pause")
		}
	}
	r.Pause = append(r.Pause, Pause{Start: start, End: end, Note: note})
	return r.Pause[len(r.Pause)-1], nil
}

// PopPause pops the last pause
func (r *Record) PopPause() (Pause, bool) {
	if len(r.Pause) == 0 {
		return Pause{}, false
	}
	p := r.Pause[len(r.Pause)-1]
	r.Pause = r.Pause[:len(r.Pause)-1]
	return p, true
}

// EndPause closes the last, open pause
func (r *Record) EndPause(t time.Time) (Pause, error) {
	if len(r.Pause) == 0 {
		return Pause{}, fmt.Errorf("no pause to end")
	}
	if !r.Pause[len(r.Pause)-1].End.IsZero() {
		return Pause{}, fmt.Errorf("last pause is already ended")
	}
	r.Pause[len(r.Pause)-1].End = t
	return r.Pause[len(r.Pause)-1], nil
}

// SaveRecord saves a record to disk
func (t *Track) SaveRecord(record *Record, force bool) error {
	path := t.RecordPath(record.Start)
	if !force && fs.FileExists(path) {
		return fmt.Errorf("record already exists")
	}
	dir := t.RecordDir(record.Start)
	err := fs.CreateDir(dir)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	defer file.Close()

	if err != nil {
		return err
	}

	bytes := SerializeRecord(record, util.NoTime)

	_, err = fmt.Fprintf(file, "%s Record %s\n", CommentPrefix, record.Start.Format(util.DateTimeFormat))
	if err != nil {
		return err
	}

	_, err = file.WriteString(bytes)

	return err
}

// DeleteRecord deletes a record
func (t *Track) DeleteRecord(record *Record) error {
	path := t.RecordPath(record.Start)
	if !fs.FileExists(path) {
		return fmt.Errorf("record does not exist")
	}
	err := os.Remove(path)
	if err != nil {
		return err
	}
	dayDir := filepath.Dir(path)
	empty, err := fs.DirIsEmpty(dayDir)
	if err != nil {
		return err
	}
	if empty {
		os.Remove(dayDir)
		monthDir := filepath.Dir(dayDir)
		empty, err := fs.DirIsEmpty(monthDir)
		if err != nil {
			return err
		}
		if empty {
			os.Remove(monthDir)
			yearDir := filepath.Dir(monthDir)
			empty, err := fs.DirIsEmpty(yearDir)
			if err != nil {
				return err
			}
			if empty {
				os.Remove(yearDir)

			}
		}
	}
	return nil
}

// LoadRecord loads a record
func (t *Track) LoadRecord(tm time.Time) (Record, error) {
	path := t.RecordPath(tm)
	file, err := os.ReadFile(path)
	if err != nil {
		if _, ok := err.(*os.PathError); ok {
			return Record{}, ErrRecordNotFound
		}
		return Record{}, err
	}

	record, err := DeserializeRecord(string(file), tm)
	if err != nil {
		return Record{}, err
	}

	return record, nil
}

// LoadAllRecords loads all records
func (t *Track) LoadAllRecords() ([]Record, error) {
	return t.LoadAllRecordsFiltered(NewFilter([]func(*Record) bool{}, util.NoTime, util.NoTime))
}

// LoadAllRecordsFiltered loads all records
func (t *Track) LoadAllRecordsFiltered(filters FilterFunctions) ([]Record, error) {
	fn, results, _ := t.AllRecordsFiltered(filters, false)
	go fn()

	var records []Record
	for res := range results {
		if res.Err != nil {
			return records, res.Err
		}
		records = append(records, res.Record)
	}

	return records, nil
}

// AllRecordsFiltered is an async version of LoadAllRecordsFiltered
func (t *Track) AllRecordsFiltered(filters FilterFunctions, reversed bool) (func(), chan FilterResult, chan struct{}) {
	results := make(chan FilterResult, 32)
	stop := make(chan struct{})

	return func() {
		defer close(results)

		path := t.RecordsDir()

		yearDirs, err := ioutil.ReadDir(path)
		if err != nil {
			results <- FilterResult{Record{}, err}
			return
		}
		if reversed {
			util.Reverse(yearDirs)
		}

		for _, yearDir := range yearDirs {
			if !yearDir.IsDir() {
				continue
			}
			year, err := strconv.Atoi(yearDir.Name())
			if err != nil {
				results <- FilterResult{Record{}, err}
				return
			}
			if !filters.Start.IsZero() && year < filters.Start.Year() {
				continue
			}
			if !filters.End.IsZero() && year > filters.End.Year() {
				continue
			}

			monthDirs, err := ioutil.ReadDir(filepath.Join(path, yearDir.Name()))

			if reversed {
				util.Reverse(monthDirs)
			}
			if err != nil {
				results <- FilterResult{Record{}, err}
				return
			}

			for _, monthDir := range monthDirs {
				if !monthDir.IsDir() {
					continue
				}
				month, err := strconv.Atoi(monthDir.Name())
				if err != nil {
					results <- FilterResult{Record{}, err}
					return
				}

				dayDirs, err := ioutil.ReadDir(filepath.Join(path, yearDir.Name(), monthDir.Name()))
				if err != nil {
					results <- FilterResult{Record{}, err}
					return
				}

				if reversed {
					util.Reverse(dayDirs)
				}
				for _, dayDir := range dayDirs {
					if !dayDir.IsDir() {
						continue
					}
					day, err := strconv.Atoi(dayDir.Name())
					if err != nil {
						results <- FilterResult{Record{}, err}
						return
					}

					date := util.Date(year, time.Month(month), day)
					if !filters.Start.IsZero() && date.Before(util.ToDate(filters.Start)) {
						continue
					}
					if !filters.End.IsZero() && date.After(filters.End) {
						continue
					}

					recs, err := t.LoadDateRecordsFiltered(date, filters)
					if err != nil {
						results <- FilterResult{Record{}, err}
						return
					}

					if reversed {
						util.Reverse(recs)
					}
					for _, rec := range recs {
						select {
						case <-stop:
							return
						case results <- FilterResult{rec, nil}:
						}
					}
				}
			}
		}
	}, results, stop
}

// LoadDateRecords loads all records for the given date
func (t *Track) LoadDateRecords(date time.Time) ([]Record, error) {
	return t.LoadDateRecordsFiltered(date, FilterFunctions{})
}

// LoadDateRecordsExact loads all records for the given date, including those starting the das before
func (t *Track) LoadDateRecordsExact(date time.Time) ([]Record, error) {
	date = util.ToDate(date)
	dateBefore := date.Add(-24 * time.Hour)
	dateAfter := date.Add(24 * time.Hour)

	filters := FilterFunctions{
		[]FilterFunction{FilterByTime(date, dateAfter)},
		util.NoTime,
		util.NoTime,
	}

	records, err := t.LoadDateRecordsFiltered(dateBefore, filters)
	if err != nil && !errors.Is(err, ErrNoRecords) {
		return nil, err
	}
	records2, err := t.LoadDateRecordsFiltered(date, filters)
	if err != nil && !errors.Is(err, ErrNoRecords) {
		return nil, err
	}
	records = append(records, records2...)

	if len(records) == 0 {
		return nil, ErrNoRecords
	}
	return records, nil
}

// LoadDateRecordsFiltered loads all records for the given date string/directory
func (t *Track) LoadDateRecordsFiltered(date time.Time, filters FilterFunctions) ([]Record, error) {
	subPath := t.RecordDir(date)

	info, err := os.Stat(subPath)
	if err != nil {
		return nil, ErrNoRecords
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("'%s' is not a directory", info.Name())
	}

	var records []Record

	files, err := ioutil.ReadDir(subPath)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		tm, err := fileToTime(date, file.Name())
		record, err := t.LoadRecord(tm)
		if err != nil {
			return nil, err
		}
		if Filter(&record, filters) {
			records = append(records, record)
		}
	}

	return records, nil
}

// FindLatestRecord loads the latest record for the given condition. Returns a nil reference if no record is found.
func (t *Track) FindLatestRecord(cond FilterFunction) (*Record, error) {
	fn, results, stop := t.AllRecordsFiltered(
		FilterFunctions{[]FilterFunction{cond}, util.NoTime, util.NoTime},
		true, // reversed order to find latest record of project
	)
	go fn()

	for res := range results {
		if res.Err != nil {
			return nil, res.Err
		}
		close(stop)
		return &res.Record, nil
	}
	return nil, nil
}

// LatestRecord loads the latest record. Returns a nil reference if no record is found.
func (t *Track) LatestRecord() (*Record, error) {
	records := t.RecordsDir()
	yearPath, year, err := fs.FindLatests(records, true)
	if err != nil {
		if errors.Is(err, fs.ErrNoFiles) {
			return nil, nil
		}
		return nil, err
	}
	monthPath, month, err := fs.FindLatests(yearPath, true)
	if err != nil {
		if errors.Is(err, fs.ErrNoFiles) {
			return nil, nil
		}
		return nil, err
	}
	dayPath, day, err := fs.FindLatests(monthPath, true)
	if err != nil {
		if errors.Is(err, fs.ErrNoFiles) {
			return nil, nil
		}
		return nil, err
	}
	_, record, err := fs.FindLatests(dayPath, false)
	if err != nil {
		if errors.Is(err, fs.ErrNoFiles) {
			return nil, nil
		}
		return nil, err
	}

	tm, err := pathToTime(year, month, day, record)
	if err != nil {
		return nil, err
	}
	rec, err := t.LoadRecord(tm)
	if err != nil {
		return nil, err
	}

	return &rec, nil
}

// OpenRecord returns the open record if any. Returns a nil reference if no open record is found.
func (t *Track) OpenRecord() (*Record, error) {
	latest, err := t.LatestRecord()
	if err != nil {
		if err == fs.ErrNoFiles {
			return nil, nil
		}
		return nil, err
	}
	if latest == nil {
		return nil, nil
	}
	if latest.HasEnded() {
		return nil, nil
	}
	return latest, nil
}

// StartRecord starts and saves a record
func (t *Track) StartRecord(project, note string, tags []string, start time.Time) (Record, error) {
	record := Record{
		Project: project,
		Note:    note,
		Tags:    tags,
		Start:   start,
		End:     util.NoTime,
	}

	return record, t.SaveRecord(&record, false)
}

// StopRecord stops and saves the current record
func (t *Track) StopRecord(end time.Time) (*Record, error) {
	record, err := t.OpenRecord()
	if err != nil {
		return record, err
	}
	if record == nil {
		return record, fmt.Errorf("no running record")
	}

	record.End = end
	for len(record.Pause) > 0 {
		idx := len(record.Pause) - 1
		if record.Pause[idx].End.IsZero() || record.Pause[idx].End.After(end) {
			record.End = record.Pause[idx].Start
			record.Pause = record.Pause[:idx]
		} else {
			break
		}
	}

	err = t.SaveRecord(record, true)
	if err != nil {
		return record, err
	}
	return record, nil
}

// ExtractTagsSlice extracts elements with the tag prefix
func ExtractTagsSlice(tokens []string) []string {
	var result []string
	mapped := make(map[string]bool)
	for _, token := range tokens {
		subTokens := strings.Split(token, " ")
		for _, subToken := range subTokens {
			if strings.HasPrefix(subToken, TagPrefix) {
				if _, ok := mapped[subToken]; !ok {
					mapped[subToken] = true
					result = append(result, strings.TrimPrefix(subToken, TagPrefix))
				}
			}
		}
	}
	return result
}

// ExtractTags extracts elements with the tag prefix
func ExtractTags(text string) []string {
	var result []string
	mapped := make(map[string]bool)
	subTokens := strings.Split(text, " ")
	for _, subToken := range subTokens {
		if strings.HasPrefix(subToken, TagPrefix) {
			if _, ok := mapped[subToken]; !ok {
				mapped[subToken] = true
				result = append(result, strings.TrimPrefix(subToken, TagPrefix))
			}
		}
	}
	return result
}

func pathToTime(y, m, d, file string) (time.Time, error) {
	return time.ParseInLocation(
		util.FileDateTimeFormat,
		fmt.Sprintf("%s-%s-%s %s", y, m, d, strings.Split(file, ".")[0]),
		time.Local,
	)
}

func fileToTime(date time.Time, file string) (time.Time, error) {
	t, err := time.ParseInLocation(util.FileTimeFormat, strings.Split(file, ".")[0], time.Local)
	if err != nil {
		return util.NoTime, err
	}
	return util.DateAndTime(date, t), nil
}
