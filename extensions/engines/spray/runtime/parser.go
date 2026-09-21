package sprayruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	enginecontract "github.com/yyhuni/lunafox/engines/spray/contract"
)

// sprayRecord mirrors the spray v0.3.x file-output JSON schema
// (core/runner.go writes one parsers.SprayResult JSON object per line).
type sprayRecord struct {
	Number      int    `json:"number"`
	IsValid     bool   `json:"valid"`
	IsFuzzy     bool   `json:"fuzzy"`
	UrlString   string `json:"url"`
	Path        string `json:"path"`
	Host        string `json:"host"`
	BodyLength  int    `json:"body_length"`
	Status      int    `json:"status"`
	Spended     int64  `json:"spend"`
	ContentType string `json:"content_type"`
	Title       string `json:"title"`
	Frameworks  map[string]struct {
		Name string `json:"name"`
	} `json:"frameworks"`
	// extracts is an array of extractor objects whose shape is tool-owned;
	// the engine does not map it to canonical results, so it stays raw.
	Extracteds json.RawMessage `json:"extracts"`
	ErrString  string          `json:"error"`
}

// ParseSprayRecord converts one decoded spray record into canonical typed
// results. Every valid or fuzzy record contributes one Directory observation;
// records carrying frameworks additionally contribute one WebsiteTechnology.
func ParseSprayRecord(record sprayRecord) ([]enginecontract.Directory, []enginecontract.WebsiteTechnology, error) {
	if !record.IsValid && !record.IsFuzzy {
		return nil, nil, nil
	}
	if record.UrlString == "" {
		return nil, nil, errors.New("spray record url is required")
	}
	if !strings.HasPrefix(record.UrlString, "http://") && !strings.HasPrefix(record.UrlString, "https://") {
		return nil, nil, fmt.Errorf("spray record url %q must begin with http:// or https://", record.UrlString)
	}
	var directories []enginecontract.Directory
	directories = append(directories, enginecontract.Directory{
		URL:           record.UrlString,
		Status:        record.Status,
		ContentLength: int64(record.BodyLength),
		ContentType:   record.ContentType,
		Duration:      record.Spended,
	})
	var technologies []enginecontract.WebsiteTechnology
	if len(record.Frameworks) > 0 {
		names := make([]string, 0, len(record.Frameworks))
		for key, framework := range record.Frameworks {
			name := framework.Name
			if name == "" {
				name = key
			}
			if name != "" {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			technologies = append(technologies, enginecontract.WebsiteTechnology{
				URL:  record.UrlString,
				Tech: names,
			})
		}
	}
	return directories, technologies, nil
}

// sprayParseSummary counts the consumed raw records for progress reporting.
type sprayParseSummary struct {
	Records      uint64
	Directories  uint64
	Technologies uint64
}

// parseSprayOutput streams the spray JSONL artifact, converts records, and
// submits typed results through the canonical ports. URL-level dedup keeps the
// first observation per result type.
func parseSprayOutput(ctx context.Context, artifact io.Reader, results enginecontract.Results) (sprayParseSummary, error) {
	summary := sprayParseSummary{}
	if results.Directories == nil {
		return summary, errors.New("typed Directory result port is required")
	}
	directoryCh := make(chan enginecontract.Directory, 64)
	directoryDone := make(chan error, 1)
	go func() {
		directoryDone <- results.Directories.Submit(ctx, directoryCh)
	}()
	technologyCh := make(chan enginecontract.WebsiteTechnology, 64)
	technologyDone := make(chan error, 1)
	if results.WebsiteTechnologies != nil {
		go func() {
			technologyDone <- results.WebsiteTechnologies.Submit(ctx, technologyCh)
		}()
	} else {
		close(technologyCh)
		technologyDone <- nil
	}

	seenDirectory := make(map[string]struct{})
	seenTechnology := make(map[string]struct{})
	scanner := bufio.NewScanner(artifact)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			close(directoryCh)
			close(technologyCh)
			return summary, err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record sprayRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			close(directoryCh)
			close(technologyCh)
			return summary, fmt.Errorf("decode spray record %d: %w", summary.Records+1, err)
		}
		summary.Records++
		directories, technologies, err := ParseSprayRecord(record)
		if err != nil {
			close(directoryCh)
			close(technologyCh)
			return summary, fmt.Errorf("convert spray record %d: %w", summary.Records, err)
		}
		for _, directory := range directories {
			if _, duplicate := seenDirectory[directory.URL]; duplicate {
				continue
			}
			seenDirectory[directory.URL] = struct{}{}
			select {
			case directoryCh <- directory:
				summary.Directories++
			case <-ctx.Done():
				close(directoryCh)
				close(technologyCh)
				return summary, ctx.Err()
			}
		}
		for _, technology := range technologies {
			if _, duplicate := seenTechnology[technology.URL]; duplicate {
				continue
			}
			seenTechnology[technology.URL] = struct{}{}
			select {
			case technologyCh <- technology:
				summary.Technologies++
			case <-ctx.Done():
				close(directoryCh)
				close(technologyCh)
				return summary, ctx.Err()
			}
		}
	}
	if err := scanner.Err(); err != nil {
		close(directoryCh)
		close(technologyCh)
		return summary, fmt.Errorf("read spray artifact: %w", err)
	}
	close(directoryCh)
	close(technologyCh)
	if err := <-directoryDone; err != nil {
		return summary, fmt.Errorf("submit Directory results: %w", err)
	}
	if err := <-technologyDone; err != nil {
		return summary, fmt.Errorf("submit WebsiteTechnology results: %w", err)
	}
	return summary, nil
}
