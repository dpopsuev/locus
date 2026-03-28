package history

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/cache"
)

// Sentinel errors for history operations.
var (
	ErrNoHistory       = errors.New("no history")
	ErrIndexOutOfRange = errors.New("index out of range")
	ErrReportNotFound  = errors.New("cached report not found (cache may have been pruned)")
)

// Source identifies how a scan was triggered.
type Source string

const (
	Local  Source = "local"
	Remote Source = "remote"
)

// Entry is the full record written per scan (includes the report for GetReport).
type Entry struct {
	Timestamp  time.Time `json:"timestamp"`
	HeadSHA    string    `json:"head_sha"`
	Source     Source    `json:"source"`
	RepoPath   string    `json:"repo_path"`
	Components int       `json:"components"`
	Edges      int       `json:"edges"`
}

// EntrySummary is a lightweight view of an Entry for listing.
type EntrySummary struct {
	Timestamp  time.Time `json:"timestamp"`
	HeadSHA    string    `json:"head_sha"`
	Source     Source    `json:"source"`
	RepoPath   string    `json:"repo_path"`
	Components int       `json:"components"`
	Edges      int       `json:"edges"`
}

// DefaultHistoryDir returns ~/.locus/history.
func DefaultHistoryDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".locus", "history")
}

// Record appends a history entry to the JSONL file and stores the full report
// in the scan cache for later retrieval.
func Record(sc *cache.ScanCache, historyDir string, source Source, repoPath, headSHA string, report *arch.ContextReport) error {
	if err := sc.Put(repoPath, headSHA, report); err != nil {
		return fmt.Errorf("cache report: %w", err)
	}

	entry := Entry{
		Timestamp:  time.Now(),
		HeadSHA:    headSHA,
		Source:     source,
		RepoPath:   repoPath,
		Components: len(report.Architecture.Services),
		Edges:      len(report.Architecture.Edges),
	}

	p := historyPath(historyDir, repoPath)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

// List returns the most recent history entries for a repo path.
func List(historyDir, repoPath string, limit int) ([]EntrySummary, error) {
	entries, err := readEntries(historyDir, repoPath)
	if err != nil {
		return nil, err
	}

	summaries := make([]EntrySummary, 0, len(entries))
	for _, e := range entries {
		summaries = append(summaries, EntrySummary(e))
	}

	if limit > 0 && len(summaries) > limit {
		summaries = summaries[len(summaries)-limit:]
	}
	return summaries, nil
}

// GetReport retrieves the full report for a specific history index.
// Negative indices count from the end (-1 = latest, -2 = previous).
func GetReport(sc *cache.ScanCache, historyDir, repoPath string, index int) (*arch.ContextReport, error) {
	entries, err := readEntries(historyDir, repoPath)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w for %s", ErrNoHistory, repoPath)
	}

	if index < 0 {
		index = len(entries) + index
	}
	if index < 0 || index >= len(entries) {
		return nil, fmt.Errorf("%w: %d (have %d entries)", ErrIndexOutOfRange, index, len(entries))
	}

	sha := entries[index].HeadSHA
	report, hit, err := sc.Get(repoPath, sha)
	if err != nil {
		return nil, fmt.Errorf("read cached report for %s: %w", sha, err)
	}
	if !hit {
		return nil, fmt.Errorf("%w: %s at %s", ErrReportNotFound, repoPath, sha)
	}
	return report, nil
}

func readEntries(historyDir, repoPath string) ([]Entry, error) {
	p := historyPath(historyDir, repoPath)
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}

func historyPath(historyDir, repoPath string) string {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		abs = repoPath
	}
	h := sha256.Sum256([]byte(abs))
	return filepath.Join(historyDir, fmt.Sprintf("%x.jsonl", h[:8]))
}
