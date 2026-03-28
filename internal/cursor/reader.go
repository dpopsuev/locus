package cursor

import (
	"os"
	"path/filepath"
	"strings"
)

type Rule struct {
	Path        string   `json:"path"`
	Description string   `json:"description,omitempty"`
	AlwaysApply bool     `json:"always_apply,omitempty"`
	Globs       []string `json:"globs,omitempty"`
	Body        string   `json:"body"`
}

type Skill struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Body        string `json:"body"`
}

func ReadRules(root string) ([]Rule, error) {
	rulesDir := filepath.Join(root, ".cursor", "rules")
	if _, err := os.Stat(rulesDir); os.IsNotExist(err) {
		return nil, nil
	}

	var rules []Rule
	err := filepath.WalkDir(rulesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".mdc") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		r := Rule{Path: rel}
		fm, body := parseFrontmatter(string(data))
		r.Body = strings.TrimSpace(body)
		r.Description = fm["description"]
		if v, ok := fm["alwaysApply"]; ok && (v == "true" || v == "True") {
			r.AlwaysApply = true
		}
		if g, ok := fm["globs"]; ok {
			r.Globs = parseYAMLList(g)
		}
		rules = append(rules, r)
		return nil
	})
	return rules, err
}

func ReadSkills(root string) ([]Skill, error) {
	skillsDir := filepath.Join(root, ".cursor", "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}
	skills := make([]Skill, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillFile := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(root, skillFile)
		sk := Skill{Path: rel}
		fm, body := parseFrontmatter(string(data))
		sk.Body = strings.TrimSpace(body)
		sk.Name = fm["name"]
		sk.Description = fm["description"]
		skills = append(skills, sk)
	}
	return skills, nil
}

func parseFrontmatter(content string) (fm map[string]string, body string) {
	fm = map[string]string{}
	if !strings.HasPrefix(content, "---") {
		return fm, content
	}

	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return fm, content
	}
	fmBlock := rest[:idx]
	body = rest[idx+4:]

	var currentKey string
	var listBuf []string
	inList := false

	for _, line := range strings.Split(fmBlock, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if inList {
			if strings.HasPrefix(trimmed, "- ") {
				val := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
				val = strings.Trim(val, `"'`)
				listBuf = append(listBuf, val)
				continue
			}
			fm[currentKey] = strings.Join(listBuf, "\n")
			inList = false
			listBuf = nil
		}

		if i := strings.Index(trimmed, ":"); i > 0 {
			key := strings.TrimSpace(trimmed[:i])
			val := strings.TrimSpace(trimmed[i+1:])
			currentKey = key
			if val == "" {
				inList = true
				listBuf = nil
				continue
			}
			fm[key] = strings.Trim(val, `"'`)
		}
	}

	if inList && currentKey != "" {
		fm[currentKey] = strings.Join(listBuf, "\n")
	}

	return fm, body
}

func parseYAMLList(raw string) []string {
	var result []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
