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
	"gopkg.in/yaml.v3"
)

// TagPrefix denotes tags in record notes
const TagPrefix = "+"

var (
	// ErrNoRecords is an error for no records found for a date
	ErrNoRecords = errors.New("no records for date")
)

// Record holds and manipulates data for a record
type Record struct {
	Project string
	Start   time.Time
	End     time.Time
	Note    string
	Tags    []string
}

type yamlRecord struct {
	Project string
	Start   util.Time
	End     util.Time
	Note    string
	Tags    []string
}

// MarshalYAML converts a Record to YAML bytes
func (r *Record) MarshalYAML() (interface{}, error) {
	return &yamlRecord{
		Project: r.Project,
		Note:    r.Note,
		Tags:    r.Tags,
		Start:   util.Time(r.Start),
		End:     util.Time(r.End),
	}, nil
}

// UnmarshalYAML converts YAML bytes to a Record
func (r *Record) UnmarshalYAML(value *yaml.Node) error {
	rec := yamlRecord{}
	if err := value.Decode(&rec); err != nil {
		return err
	}
	r.Project = rec.Project
	r.Note = rec.Note
	r.Tags = rec.Tags
	r.Start = time.Time(rec.Start)
	r.End = time.Time(rec.End)

	return nil
}

// HasEnded reports whether the record has an end time
func (r Record) HasEnded() bool {
	return !r.End.IsZero()
}

// Duration reports the duration of a record
func (r Record) Duration() time.Duration {
	t := r.End
	if t.IsZero() {
		t = time.Now()
	}
	return t.Sub(r.Start)
}

// RecordsDir returns the records storage directory
func (t *Track) RecordsDir() string {
	return filepath.Join(fs.RootDir(), t.Workspace(), fs.RecordsDirName())
}

// WorkspaceRecordsDir returns the records storage directory for the given workspace
func (t *Track) WorkspaceRecordsDir(ws string) string {
	return filepath.Join(fs.RootDir(), ws, fs.RecordsDirName())
}

// RecordPath returns the full path for a record
func (t *Track) RecordPath(tm time.Time) string {
	return filepath.Join(
		t.RecordDir(tm),
		fmt.Sprintf("%s.yml", tm.Format(util.FileTimeFormat)),
	)
}

// RecordDir returns the directory path for a record
func (t *Track) RecordDir(tm time.Time) string {
	return filepath.Join(
		t.RecordsDir(),
		fmt.Sprintf("%04d", tm.Year()),
		fmt.Sprintf("%02d", int(tm.Month())),
		fmt.Sprintf("%02d", tm.Day()),
	)
}

// SaveRecord saves a record to disk
func (t *Track) SaveRecord(record Record, force bool) error {
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

	bytes, err := yaml.Marshal(&record)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(file, "# Record %s\n\n", record.Start.Format(util.DateTimeFormat))
	if err != nil {
		return err
	}

	_, err = file.Write(bytes)

	return err
}

// DeleteRecord deletes a record
func (t *Track) DeleteRecord(record Record) error {
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

// LoadRecordByTime loads a record
func (t *Track) LoadRecordByTime(tm time.Time) (Record, error) {
	path := t.RecordPath(tm)
	return t.LoadRecord(path)
}

// LoadRecord loads a record
func (t *Track) LoadRecord(path string) (Record, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}

	var record Record

	if err := yaml.Unmarshal(file, &record); err != nil {
		return Record{}, err
	}

	return record, nil
}

// LoadAllRecords loads all records
func (t *Track) LoadAllRecords() ([]Record, error) {
	return t.LoadAllRecordsFiltered([]func(*Record) bool{})
}

// LoadAllRecordsFiltered loads all records
func (t *Track) LoadAllRecordsFiltered(filters FilterFunctions) ([]Record, error) {
	path := t.RecordsDir()

	var records []Record

	yearDirs, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}

	for _, yearDir := range yearDirs {
		if !yearDir.IsDir() {
			continue
		}
		year, err := strconv.Atoi(yearDir.Name())
		if err != nil {
			return nil, err
		}
		monthDirs, err := ioutil.ReadDir(filepath.Join(path, yearDir.Name()))
		if err != nil {
			return nil, err
		}

		for _, monthDir := range monthDirs {
			if !monthDir.IsDir() {
				continue
			}
			month, err := strconv.Atoi(monthDir.Name())
			if err != nil {
				return nil, err
			}

			dayDirs, err := ioutil.ReadDir(filepath.Join(path, yearDir.Name(), monthDir.Name()))
			if err != nil {
				return nil, err
			}

			for _, dayDir := range dayDirs {
				if !dayDir.IsDir() {
					continue
				}
				day, err := strconv.Atoi(dayDir.Name())
				if err != nil {
					return nil, err
				}

				recs, err := t.LoadDateRecordsFiltered(util.Date(year, time.Month(month), day), filters)
				if err != nil {
					return nil, err
				}
				records = append(records, recs...)
			}
		}
	}

	return records, nil
}

// FilterResult contains a Report or an error from async filtering
type FilterResult struct {
	Record Record
	Err    error
}

// AllRecordsFiltered is an async version of LoadAllRecordsFiltered
func (t *Track) AllRecordsFiltered(filters FilterFunctions) (func(), chan FilterResult) {
	results := make(chan FilterResult, 32)

	return func() {
		path := t.RecordsDir()

		yearDirs, err := ioutil.ReadDir(path)
		if err != nil {
			results <- FilterResult{Record{}, err}
			return
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
			monthDirs, err := ioutil.ReadDir(filepath.Join(path, yearDir.Name()))
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

				for _, dayDir := range dayDirs {
					if !dayDir.IsDir() {
						continue
					}
					day, err := strconv.Atoi(dayDir.Name())
					if err != nil {
						results <- FilterResult{Record{}, err}
						return
					}

					recs, err := t.LoadDateRecordsFiltered(util.Date(year, time.Month(month), day), filters)
					if err != nil {
						results <- FilterResult{Record{}, err}
						return
					}
					for _, rec := range recs {
						results <- FilterResult{rec, nil}
					}
				}
			}
		}
		close(results)
	}, results
}

// LoadDateRecords loads all records for the given date string/directory
func (t *Track) LoadDateRecords(date time.Time) ([]Record, error) {
	return t.LoadDateRecordsFiltered(date, []func(*Record) bool{})
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

		record, err := t.LoadRecord(filepath.Join(subPath, file.Name()))
		if err != nil {
			return nil, err
		}
		if Filter(&record, filters) {
			records = append(records, record)
		}
	}

	return records, nil
}

// LatestRecord loads the latest record
func (t *Track) LatestRecord() (Record, error) {
	records := t.RecordsDir()
	year, err := fs.FindLatests(records, true)
	if err != nil {
		return Record{}, err
	}
	month, err := fs.FindLatests(year, true)
	if err != nil {
		return Record{}, err
	}
	day, err := fs.FindLatests(month, true)
	if err != nil {
		return Record{}, err
	}
	record, err := fs.FindLatests(day, false)
	if err != nil {
		return Record{}, err
	}

	rec, err := t.LoadRecord(record)
	if err != nil {
		return Record{}, err
	}

	return rec, nil
}

// OpenRecord returns the open record if any
func (t *Track) OpenRecord() (rec Record, ok bool) {
	latest, err := t.LatestRecord()
	if err != nil {
		if err == fs.ErrNoFiles {
			return Record{}, false
		}
		return Record{}, false
	}
	if latest.HasEnded() {
		return Record{}, false
	}
	return latest, true
}

// StartRecord starts and saves a record
func (t *Track) StartRecord(project, note string, tags []string, start time.Time) (Record, error) {
	record := Record{
		Project: project,
		Note:    note,
		Tags:    tags,
		Start:   start,
		End:     time.Time{},
	}

	return record, t.SaveRecord(record, false)
}

// StopRecord stops and saves the current record
func (t *Track) StopRecord(end time.Time) (Record, error) {
	record, ok := t.OpenRecord()
	if !ok {
		return record, fmt.Errorf("no running record")
	}

	record.End = end

	err := t.SaveRecord(record, true)
	if err != nil {
		return record, err
	}
	return record, nil
}

// ExtractTags extracts elements with the tag prefix
func (t *Track) ExtractTags(tokens []string) []string {
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
