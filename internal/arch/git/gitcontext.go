package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// MaxAuthorsPerPackage is the max authors returned per package.
	MaxAuthorsPerPackage = 5
	// MaxFileHotSpots is the max file hotspots returned.
	MaxFileHotSpots = 50
)

// PackageCommit represents a single git commit associated with a package.
type PackageCommit struct {
	Package string    `json:"package"`
	Hash    string    `json:"hash"`
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
	Message string    `json:"message"`
}

// Author represents a contributor with their commit count.
type Author struct {
	Name    string `json:"name"`
	Commits int    `json:"commits"`
}

// HotFile identifies a frequently-changed individual file.
type HotFile struct {
	Path    string `json:"path"`
	Package string `json:"package"`
	Changes int    `json:"changes"`
}

// RecentCommits returns per-package recent commits from git history.
func RecentCommits(root string, days int, modPath string) []PackageCommit {
	if days <= 0 {
		return nil
	}

	sinceArg := fmt.Sprintf("--since=%d.days.ago", days)
	cmd := exec.Command("git", "log", "--format=%H|%an|%aI|%s", "--name-only", sinceArg)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	absRoot, _ := filepath.Abs(root)
	lines := strings.Split(string(out), "\n")
	commits := make([]PackageCommit, 0, len(lines))
	var currentHash, currentAuthor, currentMsg string
	var currentDate time.Time

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if parts := strings.SplitN(line, "|", 4); len(parts) == 4 && len(parts[0]) == 40 {
			currentHash = parts[0]
			currentAuthor = parts[1]
			currentDate, _ = time.Parse(time.RFC3339, parts[2])
			currentMsg = parts[3]
			continue
		}

		if currentHash == "" {
			continue
		}

		dir := filepath.Dir(line)
		if dir == "." {
			continue
		}
		full := filepath.Join(absRoot, dir)
		rel, err := filepath.Rel(absRoot, full)
		if err != nil {
			continue
		}
		pkg := filepath.ToSlash(rel)

		commits = append(commits, PackageCommit{
			Package: pkg,
			Hash:    currentHash[:8],
			Author:  currentAuthor,
			Date:    currentDate,
			Message: currentMsg,
		})
	}

	seen := make(map[string]bool)
	deduped := make([]PackageCommit, 0, len(commits))
	for _, c := range commits {
		key := c.Hash + "|" + c.Package
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, c)
	}

	sort.Slice(deduped, func(i, j int) bool { return deduped[i].Date.After(deduped[j].Date) })
	return deduped
}

// AuthorOwnership returns per-package top contributors from git history.
func AuthorOwnership(root, modPath string) map[string][]Author {
	cmd := exec.Command("git", "log", "--format=%an", "--name-only")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	absRoot, _ := filepath.Abs(root)
	pkgAuthors := make(map[string]map[string]int)
	var currentAuthor string

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if !strings.Contains(line, "/") && !strings.Contains(line, ".") {
			currentAuthor = line
			continue
		}

		if currentAuthor == "" {
			continue
		}

		dir := filepath.Dir(line)
		if dir == "." {
			continue
		}
		full := filepath.Join(absRoot, dir)
		rel, err := filepath.Rel(absRoot, full)
		if err != nil {
			continue
		}
		pkg := filepath.ToSlash(rel)

		if pkgAuthors[pkg] == nil {
			pkgAuthors[pkg] = make(map[string]int)
		}
		pkgAuthors[pkg][currentAuthor]++
	}

	result := make(map[string][]Author, len(pkgAuthors))
	for pkg, authors := range pkgAuthors {
		var list []Author
		for name, count := range authors {
			list = append(list, Author{Name: name, Commits: count})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Commits > list[j].Commits })
		if len(list) > MaxAuthorsPerPackage {
			list = list[:MaxAuthorsPerPackage]
		}
		result[pkg] = list
	}
	return result
}

// FileHotSpots returns the most-changed individual files from git history.
func FileHotSpots(root string, days int) []HotFile {
	if days <= 0 {
		return nil
	}

	sinceArg := fmt.Sprintf("--since=%d.days.ago", days)
	cmd := exec.Command("git", "log", "--format=", "--name-only", sinceArg)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	absRoot, _ := filepath.Abs(root)
	fileCounts := make(map[string]int)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fileCounts[line]++
	}

	files := make([]HotFile, 0, len(fileCounts))
	for path, count := range fileCounts {
		dir := filepath.Dir(path)
		if dir == "." {
			dir = ""
		}
		full := filepath.Join(absRoot, dir)
		rel, err := filepath.Rel(absRoot, full)
		if err != nil {
			rel = dir
		}
		pkg := filepath.ToSlash(rel)
		files = append(files, HotFile{Path: path, Package: pkg, Changes: count})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Changes > files[j].Changes })
	if len(files) > MaxFileHotSpots {
		files = files[:MaxFileHotSpots]
	}
	return files
}
