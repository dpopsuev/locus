package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/config"
	"github.com/dpopsuev/locus/internal/diagram"
	diagramcore "github.com/dpopsuev/locus/internal/diagram/core"
	gitpkg "github.com/dpopsuev/locus/internal/git"
	"github.com/dpopsuev/locus/internal/lint"
	locusmcp "github.com/dpopsuev/locus/internal/mcp"
	"github.com/dpopsuev/locus/internal/oculus"
	"github.com/dpopsuev/locus/internal/oculus/lsp"
	"github.com/dpopsuev/locus/internal/protocol"
	"github.com/dpopsuev/locus/internal/triage"
)

// version is set by Execute and used by commands.
var version string

// Sentinel errors for CLI validation.
var (
	errComponentRequired = errors.New("--component is required")
	errNoIntent          = errors.New("provide an intent string, --list, or --category")
	errUnknownFormat     = errors.New("unknown format")
)

const (
	formatJSON    = "json"
	formatMermaid = "mermaid"

	// Slog attribute keys.
	logKeyVersion   = "version"
	logKeyTransport = "transport"
	logKeyAddr      = "addr"
	logKeyLevel     = "level"
)

func newProto() *protocol.Protocol {
	return protocol.New(config.NewStore(), nil)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var rootCmd = &cobra.Command{
	Use:   "locus",
	Short: "Spatial context bus for AI agents",
	Long: `Locus scans any repository and provides structured context to AI agents:
architecture, dependency graph, git history, churn, hot spots, symbols,
.cursor/ rules and skills -- via CLI or MCP server.

No ceremony required. No config directory. Just point and go.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		initLogger()
		slog.LogAttrs(context.Background(), slog.LevelDebug, "logger initialized", slog.String(logKeyLevel, os.Getenv("LOCUS_LOG_LEVEL")))
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run:   func(cmd *cobra.Command, args []string) { fmt.Printf("locus %s\n", version) },
}

var scanFlags struct {
	format          string
	scanner         string
	depth           int
	churnDays       int
	gitDays         int
	authors         bool
	includeExternal bool
	includeTests    bool
	budget          int
}

var scanCmd = &cobra.Command{
	Use:   "scan [path]",
	Short: "Scan a repository and emit structured context",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ""
		if len(args) > 0 {
			path = args[0]
		}
		proto := newProto()
		result, err := proto.ScanProject(cmd.Context(), path, protocol.ScanOpts{
			Scanner:         scanFlags.scanner,
			Depth:           scanFlags.depth,
			ChurnDays:       scanFlags.churnDays,
			GitDays:         scanFlags.gitDays,
			Authors:         scanFlags.authors,
			IncludeExternal: scanFlags.includeExternal,
			IncludeTests:    scanFlags.includeTests,
			Budget:          scanFlags.budget,
		})
		if err != nil {
			return err
		}
		return renderReport(result.Report, scanFlags.format)
	},
}

var serveFlags struct {
	workspaces []string
	transport  string
	addr       string
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Locus MCP server (stdio or HTTP)",
	Long: `Start an MCP server that exposes codebase context and knowledge tools.

  stdio (default): reads/writes JSON-RPC over stdin/stdout.
  http:            starts a Streamable HTTP server on --addr.

Tools: scan_project, get_dependencies, get_impact, get_coupling_table,
       codograph_remote, get_codograph_history, diff_branches,
       get_cycles, render_diagram, triage.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		roots := serveFlags.workspaces
		if len(roots) == 0 {
			cwd, _ := os.Getwd()
			roots = []string{cwd}
		}
		s := config.NewStore()
		defer s.Close()

		pool := lsp.NewPool()

		ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT)
		defer stop()
		defer pool.Shutdown(ctx) //nolint:errcheck // best-effort cleanup

		srv, _ := locusmcp.NewServer(s, roots, version, pool)
		if serveFlags.transport == "http" {
			handler := sdkmcp.NewStreamableHTTPHandler(
				func(r *http.Request) *sdkmcp.Server { return srv },
				nil,
			)
			slog.LogAttrs(ctx, slog.LevelInfo, "locus server starting", slog.String(logKeyVersion, version), slog.String(logKeyTransport, "http"), slog.String(logKeyAddr, serveFlags.addr))
			server := &http.Server{Addr: serveFlags.addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
			go func() {
				<-ctx.Done()
				slog.LogAttrs(ctx, slog.LevelInfo, "shutting down")
				server.Close()
			}()
			return server.ListenAndServe()
		}
		slog.LogAttrs(ctx, slog.LevelInfo, "locus server starting", slog.String(logKeyVersion, version), slog.String(logKeyTransport, "stdio"))
		return srv.Run(ctx, &sdkmcp.StdioTransport{})
	},
}

var codographFlags struct {
	ref    string
	keep   bool
	format string
	depth  int
	budget int
}

var codographCmd = &cobra.Command{
	Use:   "codograph <url>",
	Short: "Produce a codograph from a remote GitHub repository",
	Long: `Shallow-clone a remote repo and produce the same codograph as 'locus scan'.
Supports GitHub HTTPS, SSH, and shorthand URLs:
  locus codograph github.com/org/repo
  locus codograph git@github.com:org/repo.git --ref v1.2.0`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proto := newProto()
		result, err := proto.CodographRemote(cmd.Context(), args[0], protocol.RemoteOpts{
			Ref:    codographFlags.ref,
			Keep:   codographFlags.keep,
			Depth:  codographFlags.depth,
			Budget: codographFlags.budget,
		})
		if err != nil {
			return err
		}
		return renderReport(result.Report, codographFlags.format)
	},
}

var historyFlags struct {
	last int
	diff bool
}

var historyCmd = &cobra.Command{
	Use:   "history [path]",
	Short: "List past codographs and show inter-session diffs",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ""
		if len(args) > 0 {
			path = args[0]
		}
		proto := newProto()

		if historyFlags.diff {
			d, err := proto.DiffCodographs(cmd.Context(), path)
			if err != nil {
				return err
			}
			return printJSON(d)
		}

		entries, err := proto.GetHistory(cmd.Context(), path, historyFlags.last)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Println("No codograph history found.")
			return nil
		}
		return printJSON(entries)
	},
}

var diffFlags struct {
	branchA string
	branchB string
}

var diffCmd = &cobra.Command{
	Use:   "diff [path]",
	Short: "Compare architecture between two git branches",
	Long: `Compare the architecture of two branches in a repository.
Scans each branch (cache-aware: previously scanned branches are instant hits)
and returns the structural diff.

  locus diff --branch-a main --branch-b feature-x
  locus diff /path/to/repo --branch-a release-1.0 --branch-b release-2.0`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ""
		if len(args) > 0 {
			path = args[0]
		}
		proto := newProto()
		r, err := proto.DiffBranches(cmd.Context(), path, diffFlags.branchA, diffFlags.branchB)
		if err != nil {
			return err
		}
		return printJSON(r)
	},
}

var validateFlags struct {
	desired string
	format  string
}

var validateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate live architecture against a desired-state definition",
	Long: `Parse a desired-state architecture from a mermaid or JSON file,
scan the repository, and report drift (missing/extra components and edges).

  locus validate --desired arch.mermaid
  locus validate /path/to/repo --desired arch.json --format json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ""
		if len(args) > 0 {
			path = args[0]
		}
		data, err := os.ReadFile(validateFlags.desired)
		if err != nil {
			return fmt.Errorf("read desired-state file: %w", err)
		}
		format := validateFlags.format
		if format == "" {
			if ext := validateFlags.desired; ext != "" {
				if idx := len(ext) - 1; idx > 0 {
					for i := len(ext) - 1; i >= 0; i-- {
						if ext[i] == '.' {
							format = ext[i+1:]
							break
						}
					}
				}
			}
			if format != formatJSON && format != formatMermaid && format != "md" {
				format = formatMermaid
			}
		}

		proto := newProto()
		drift, err := proto.ValidateArchitecture(cmd.Context(), path, string(data), format)
		if err != nil {
			return err
		}
		return printJSON(drift)
	},
}

var diagramFlags struct {
	diagramType  string
	scope        string
	depth        int
	topN         int
	scanner      string
	churnDays    int
	entry        string
	exportedOnly bool
	theme        string
}

var diagramCmd = &cobra.Command{
	Use:   "diagram [path]",
	Short: "Render a Mermaid diagram from repository structure",
	Long: `Produce a Mermaid diagram from a scanned repository.

Supported types:
  dependency  Flowchart of package imports (default)
  c4          C4 Component diagram with containers
  coupling    Sankey diagram of fan-in/fan-out flows
  churn       XY chart of churn over time or per component
  layers      Block diagram of detected layers
  tree        Mindmap of module hierarchy
  classes     Class/struct/interface hierarchy (Tier 2)
  sequence    Call chain from an entry point (Tier 2)
  er          Entity-relationship from struct fields (Tier 2)
  dataflow    Data flow diagram with trust boundaries (Tier 3)
  callgraph   Function call graph clustered by package (Tier 3)
  state       State machine diagram from const/iota patterns (Tier 3)

Examples:
  locus diagram .
  locus diagram /path/to/repo --type c4
  locus diagram . --type coupling --top 10
  locus diagram . --type classes --scope internal/arch
  locus diagram . --type sequence --entry main
  locus diagram . --type er
  locus diagram . --type callgraph --entry main --depth 5
  locus diagram . --type dataflow --entry main
  locus diagram . --type state`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ""
		if len(args) > 0 {
			path = args[0]
		}
		if path == "" {
			path = "."
		}
		proto := newProto()
		result, err := proto.ScanProject(cmd.Context(), path, protocol.ScanOpts{
			Scanner:   diagramFlags.scanner,
			Depth:     diagramFlags.depth,
			ChurnDays: diagramFlags.churnDays,
		})
		if err != nil {
			return err
		}

		in := diagramcore.Input{Report: result.Report, Root: path}

		if diagramFlags.diagramType == "churn" {
			hist, _ := proto.GetHistory(cmd.Context(), path, 20)
			in.History = hist
		}

		switch diagramFlags.diagramType {
		case "classes", "sequence", "er":
			in.Analyzer = oculus.NewFallback(path, nil)
		case "dataflow", "callgraph", "state":
			in.DeepAnalyzer = oculus.NewDeepFallback(path, nil)
		}

		theme := diagramFlags.theme
		if theme == "" {
			theme = os.Getenv("LOCUS_THEME")
		}
		out, err := diagram.Render(in, diagramcore.Options{
			Type:         diagramFlags.diagramType,
			Scope:        diagramFlags.scope,
			Depth:        diagramFlags.depth,
			TopN:         diagramFlags.topN,
			Entry:        diagramFlags.entry,
			ExportedOnly: diagramFlags.exportedOnly,
			Theme:        theme,
		})
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	},
}

var impactFlags struct {
	component string
}

var impactCmd = &cobra.Command{
	Use:   "impact [path]",
	Short: "Compute transitive blast radius for a component change",
	Long: `Scan the repository and compute impact of changing a component:
direct dependents, transitive dependents, blast radius %, and risk level.

  locus impact . --component internal/arch`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		if impactFlags.component == "" {
			return errComponentRequired
		}
		proto := newProto()
		r, err := proto.GetImpact(cmd.Context(), path, impactFlags.component)
		if err != nil {
			return err
		}
		return printJSON(r)
	},
}

var conventionsCmd = &cobra.Command{
	Use:   "conventions [path]",
	Short: "Detect coding conventions from the codebase",
	Long: `Scan the project and report detected conventions: naming (camelCase/snake_case),
structure (cmd/, internal/, pkg/), test patterns (*_test.go, *.spec.ts), config files.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		proto := newProto()
		r, err := proto.GetConventions(cmd.Context(), path)
		if err != nil {
			return err
		}
		return printJSON(r)
	},
}

var gapsCmd = &cobra.Command{
	Use:   "gaps [path]",
	Short: "Identify undocumented or under-tested components",
	Long: `Scan the repository and report knowledge gaps: components without tests,
without docs, or high fan-in components lacking tests (critical).`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		proto := newProto()
		r, err := proto.GetGaps(cmd.Context(), path)
		if err != nil {
			return err
		}
		return printJSON(r)
	},
}

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check the Locus runtime environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		proto := newProto()
		h := proto.Health(cmd.Context())
		for _, c := range h.Checks {
			status := "OK"
			if !c.OK {
				status = "FAIL"
			}
			detail := ""
			if c.Detail != "" {
				detail = " — " + c.Detail
			}
			fmt.Printf("  [%s] %s%s\n", status, c.Name, detail)
		}
		if !h.OK {
			fmt.Println("\nSome checks failed.")
			os.Exit(1)
		}
		fmt.Println("\nAll checks passed.")
		return nil
	},
}

var triageFlags struct {
	list     bool
	category string
}

var triageCmd = &cobra.Command{
	Use:   "triage [intent] [path]",
	Short: "Map a natural language intent to ranked Locus tools",
	Long: `Pure keyword matching — no LLM required. Returns a ranked tool chain
with parameters and rationale for a given intent.

Examples:
  locus triage "find performance bottlenecks" /path/to/repo
  locus triage --list
  locus triage --category performance`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, reg := locusmcp.NewServer(config.NewStore(), nil, version)

		if triageFlags.list {
			tools := reg.List()
			grouped := make(map[string][]triage.ToolMeta)
			for _, t := range tools {
				for _, cat := range t.Categories {
					grouped[cat] = append(grouped[cat], t)
				}
			}
			for cat, catTools := range grouped {
				fmt.Printf("\n## %s\n", cat)
				for _, t := range catTools {
					fmt.Printf("  %-25s %s\n", t.Name, t.Description)
				}
			}
			return nil
		}

		if triageFlags.category != "" {
			tools := reg.ByCategory(triageFlags.category)
			if len(tools) == 0 {
				fmt.Printf("No tools in category %q.\n", triageFlags.category)
				return nil
			}
			fmt.Printf("\n## %s\n", triageFlags.category)
			for _, t := range tools {
				fmt.Printf("  %-25s %s\n", t.Name, t.Description)
				if r, ok := t.Rationale[triageFlags.category]; ok {
					fmt.Printf("    → %s\n", r)
				}
			}
			return nil
		}

		if len(args) == 0 {
			return errNoIntent
		}

		intent := args[0]
		path := ""
		if len(args) > 1 {
			path = args[1]
		}
		result := reg.Triage(intent, path)
		return printJSON(result)
	},
}

var lintFlags struct {
	format  string
	since   string
	linters string
}

var lintCmd = &cobra.Command{
	Use:   "lint [path]",
	Short: "Run architectural linters",
	Long: `Scan a repository and run architectural linters to detect violations.
Checks hexagonal architecture, SOLID principles, pattern smells, and symbol quality.

  locus lint .
  locus lint /path/to/repo --format json
  locus lint . --linters hexa,solid
  locus lint . --since HEAD~5`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}

		proto := newProto()
		result, err := proto.ScanProject(cmd.Context(), path, protocol.ScanOpts{
			Intent: string(arch.IntentHealth),
		})
		if err != nil {
			return err
		}

		// Load optional .locus.yaml config.
		ds, err := config.LoadLocusConfig(path)
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}

		// Parse --linters flag into categories.
		var categories []lint.Category
		if lintFlags.linters != "" {
			for _, s := range strings.Split(lintFlags.linters, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					categories = append(categories, lint.Category(s))
				}
			}
		}

		// Resolve changed components for incremental mode.
		var changed []string
		if lintFlags.since != "" {
			files, gitErr := gitpkg.ChangedFilesSince(path, lintFlags.since)
			if gitErr == nil {
				changed = filesToComponents(files)
			}
		}

		lintReport := lint.Run(result.Report, lint.RunOpts{
			EnabledLinters:    categories,
			DesiredState:      ds,
			Root:              path,
			ChangedComponents: changed,
		})

		switch lintFlags.format {
		case formatJSON:
			return printJSON(lintReport)
		case "table":
			return printLintTable(lintReport)
		default:
			return printLintSummary(lintReport)
		}
	},
}

// printLintSummary prints a concise human-readable summary.
func printLintSummary(r *lint.Report) error {
	fmt.Printf("Locus Lint: %d violations (score: %.1f)\n", len(r.Violations), r.Score)

	if len(r.ByCategory) > 0 {
		fmt.Println()
		for _, cat := range []lint.Category{
			lint.CategoryHexa, lint.CategorySOLID, lint.CategoryPattern,
			lint.CategorySymbol, lint.CategoryLayer, lint.CategoryBudget,
		} {
			if n, ok := r.ByCategory[cat]; ok && n > 0 {
				fmt.Printf("  %-10s %d\n", string(cat)+":", n)
			}
		}
	}

	if !r.Clean {
		os.Exit(1)
	}
	return nil
}

// printLintTable prints violations as an aligned table.
func printLintTable(r *lint.Report) error {
	fmt.Printf("%-10s %-8s %-30s %s\n", "CATEGORY", "SEVERITY", "COMPONENT", "RULE")
	fmt.Println(strings.Repeat("-", 80))
	for _, v := range r.Violations {
		fmt.Printf("%-10s %-8s %-30s %s\n", v.Category, v.Severity, v.Component, v.Rule)
	}
	fmt.Printf("\n%d violations, score: %.1f\n", len(r.Violations), r.Score)

	if !r.Clean {
		os.Exit(1)
	}
	return nil
}

// filesToComponents deduplicates file paths into component directories.
// It strips the file basename, keeping only the directory portion.
func filesToComponents(files []string) []string {
	seen := make(map[string]bool, len(files))
	var result []string
	for _, f := range files {
		dir := filepath.Dir(f)
		if dir == "." {
			continue
		}
		if !seen[dir] {
			seen[dir] = true
			result = append(result, dir)
		}
	}
	return result
}

func init() {
	rootCmd.AddCommand(versionCmd, scanCmd, serveCmd, codographCmd, historyCmd, diffCmd, validateCmd, conventionsCmd, impactCmd, gapsCmd, healthCmd, diagramCmd, triageCmd, lintCmd)

	scanCmd.Flags().StringVar(&scanFlags.format, "format", formatJSON, "Output format: json, md, mermaid")
	scanCmd.Flags().StringVar(&scanFlags.scanner, "scanner", "auto", "Scanner: auto, go, packages, rust, typescript, composite, ctags, lsp")
	scanCmd.Flags().IntVar(&scanFlags.depth, "depth", 0, "Group namespaces by first N directory segments")
	scanCmd.Flags().IntVar(&scanFlags.churnDays, "churn-days", 30, "Overlay file churn from last N days of git history (0 = disabled)")
	scanCmd.Flags().IntVar(&scanFlags.gitDays, "git-days", 30, "Recent commits window in days")
	scanCmd.Flags().BoolVar(&scanFlags.authors, "authors", false, "Include author ownership data")
	scanCmd.Flags().BoolVar(&scanFlags.includeExternal, "include-external", false, "Include external (third-party) dependencies")
	scanCmd.Flags().BoolVar(&scanFlags.includeTests, "include-tests", false, "Include test packages")
	scanCmd.Flags().IntVar(&scanFlags.budget, "budget", 0, "Cap output to N tokens (rank by importance, 0 = unlimited)")

	serveCmd.Flags().StringArrayVar(&serveFlags.workspaces, "workspace", nil, "Workspace root paths (repeatable; defaults to cwd)")
	serveCmd.Flags().StringVar(&serveFlags.transport, "transport", envOr("LOCUS_TRANSPORT", "stdio"), "Transport type: stdio, http ($LOCUS_TRANSPORT)")
	serveCmd.Flags().StringVar(&serveFlags.addr, "addr", envOr("LOCUS_ADDR", ":8081"), "Listen address for http transport ($LOCUS_ADDR)")

	codographCmd.Flags().StringVar(&codographFlags.ref, "ref", "", "Branch or tag to clone (default: repo default branch)")
	codographCmd.Flags().BoolVar(&codographFlags.keep, "keep", false, "Keep the cloned directory after scan")
	codographCmd.Flags().StringVar(&codographFlags.format, "format", formatJSON, "Output format: json, md, mermaid")
	codographCmd.Flags().IntVar(&codographFlags.depth, "depth", 0, "Group namespaces by first N directory segments")
	codographCmd.Flags().IntVar(&codographFlags.budget, "budget", 0, "Cap output to N tokens (0 = unlimited)")

	historyCmd.Flags().IntVar(&historyFlags.last, "last", 10, "Show last N codographs")
	historyCmd.Flags().BoolVar(&historyFlags.diff, "diff", false, "Show diff between two most recent codographs")

	diffCmd.Flags().StringVar(&diffFlags.branchA, "branch-a", "", "First branch to compare")
	diffCmd.Flags().StringVar(&diffFlags.branchB, "branch-b", "", "Second branch to compare")

	impactCmd.Flags().StringVar(&impactFlags.component, "component", "", "Component to analyze (e.g. internal/arch)")

	validateCmd.Flags().StringVar(&validateFlags.desired, "desired", "", "Path to desired-state file (mermaid or JSON)")
	validateCmd.Flags().StringVar(&validateFlags.format, "format", "", "Format of desired-state file: mermaid, json (auto-detected from extension)")
	_ = validateCmd.MarkFlagRequired("desired")

	diagramCmd.Flags().StringVar(&diagramFlags.diagramType, "type", "dependency", "Diagram type: dependency, c4, coupling, churn, layers, tree, classes, sequence, er, dataflow, callgraph, state")
	diagramCmd.Flags().StringVar(&diagramFlags.scope, "scope", "", "Restrict to a single component and its neighbors")
	diagramCmd.Flags().IntVar(&diagramFlags.depth, "depth", 0, "Grouping depth (0 = use suggested)")
	diagramCmd.Flags().IntVar(&diagramFlags.topN, "top", 0, "Limit items shown (0 = all)")
	diagramCmd.Flags().StringVar(&diagramFlags.scanner, "scanner", "auto", "Scanner: auto, go, packages, rust, typescript")
	diagramCmd.Flags().IntVar(&diagramFlags.churnDays, "churn-days", 30, "Git churn window in days")
	diagramCmd.Flags().StringVar(&diagramFlags.entry, "entry", "", "Entry point function name (sequence, dataflow, callgraph)")
	diagramCmd.Flags().BoolVar(&diagramFlags.exportedOnly, "exported-only", false, "Only exported functions (callgraph)")
	diagramCmd.Flags().StringVar(&diagramFlags.theme, "theme", "", "Color theme: light, dark, natural (default: $LOCUS_THEME or natural)")

	triageCmd.Flags().BoolVar(&triageFlags.list, "list", false, "List all registered tools grouped by category")
	triageCmd.Flags().StringVar(&triageFlags.category, "category", "", "Show tools in a specific category")

	lintCmd.Flags().StringVar(&lintFlags.format, "format", "summary", "Output format: summary, table, json")
	lintCmd.Flags().StringVar(&lintFlags.since, "since", "", "Git ref for incremental mode (only violations in changed files)")
	lintCmd.Flags().StringVar(&lintFlags.linters, "linters", "", "Comma-separated linter categories: hexa,solid,pattern,symbol,layer,budget")
}

func renderReport(report *arch.ContextReport, format string) error {
	switch format {
	case formatJSON:
		data, err := arch.RenderJSON(report)
		if err != nil {
			return fmt.Errorf("render JSON: %w", err)
		}
		fmt.Println(string(data))
	case "md":
		fmt.Print(arch.RenderArchMarkdown(report.Architecture))
	case formatMermaid:
		fmt.Print(arch.RenderMermaid(report.Architecture))
	default:
		return fmt.Errorf("%w %q (use json, md, or mermaid)", errUnknownFormat, format)
	}
	return nil
}

func printJSON(v any) error {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
	return nil
}

func initLogger() {
	level := slog.LevelInfo
	if v := os.Getenv("LOCUS_LOG_LEVEL"); v != "" {
		switch strings.ToLower(v) {
		case "debug":
			level = slog.LevelDebug
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
	}
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}

// Execute sets the version string and runs the root cobra command.
func Execute(v string) error {
	version = v
	config.Version = v // BUG-30: cache busting by Locus version
	return rootCmd.Execute()
}
