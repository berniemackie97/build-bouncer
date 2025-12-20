package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	assettemplates "build-bouncer/assets/templates"
)

type configTemplate struct {
	ID      string
	File    string
	Summary string
	Flags   []string
}

var configTemplates = []configTemplate{
	{
		ID:      "manual",
		File:    "config_manual.yaml",
		Summary: "Blank boilerplate (add your own checks)",
		Flags:   []string{"manual", "custom", "blank"},
	},
	{
		ID:      "go",
		File:    "config_go.yaml",
		Summary: "Go projects (go test ./..., go vet ./...)",
		Flags:   []string{"go", "golang"},
	},
	{
		ID:      "dotnet",
		File:    "config_dotnet.yaml",
		Summary: ".NET projects (dotnet test, dotnet format)",
		Flags:   []string{"dotnet", "net", "csharp"},
	},
	{
		ID:      "node",
		File:    "config_node.yaml",
		Summary: "Node projects (npm run lint/test/build)",
		Flags:   []string{"node", "nodejs", "js", "javascript", "ts", "typescript"},
	},
	{
		ID:      "react",
		File:    "config_react.yaml",
		Summary: "React projects (npm run lint/test/build)",
		Flags:   []string{"react", "reactjs"},
	},
	{
		ID:      "vue",
		File:    "config_node.yaml",
		Summary: "Vue projects (npm run lint/test/build)",
		Flags:   []string{"vue", "vuejs"},
	},
	{
		ID:      "angular",
		File:    "config_node.yaml",
		Summary: "Angular projects (npm run lint/test/build)",
		Flags:   []string{"angular", "ng"},
	},
	{
		ID:      "svelte",
		File:    "config_node.yaml",
		Summary: "Svelte projects (npm run lint/test/build)",
		Flags:   []string{"svelte", "sveltekit"},
	},
	{
		ID:      "nextjs",
		File:    "config_node.yaml",
		Summary: "Next.js projects (npm run lint/test/build)",
		Flags:   []string{"next", "nextjs"},
	},
	{
		ID:      "nuxt",
		File:    "config_node.yaml",
		Summary: "Nuxt projects (npm run lint/test/build)",
		Flags:   []string{"nuxt", "nuxtjs"},
	},
	{
		ID:      "astro",
		File:    "config_node.yaml",
		Summary: "Astro projects (uses package.json scripts like check/build)",
		Flags:   []string{"astro"},
	},
	{
		ID:      "python",
		File:    "config_python.yaml",
		Summary: "Python projects (ruff/black/pytest)",
		Flags:   []string{"python", "py", "django", "flask", "fastapi"},
	},
	{
		ID:      "ruby",
		File:    "config_ruby.yaml",
		Summary: "Ruby projects (rubocop/rspec)",
		Flags:   []string{"ruby", "rails"},
	},
	{
		ID:      "php",
		File:    "config_php.yaml",
		Summary: "PHP projects (composer scripts)",
		Flags:   []string{"php", "laravel", "symfony"},
	},
	{
		ID:      "maven",
		File:    "config_java_maven.yaml",
		Summary: "Java projects with Maven (mvn test/package)",
		Flags:   []string{"maven", "java-maven"},
	},
	{
		ID:      "gradle",
		File:    "config_java_gradle.yaml",
		Summary: "Java projects with Gradle (gradlew test/build)",
		Flags:   []string{"gradle", "java-gradle"},
	},
	{
		ID:      "kotlin",
		File:    "config_java_gradle.yaml",
		Summary: "Kotlin projects (gradlew test/build)",
		Flags:   []string{"kotlin", "kt"},
	},
	{
		ID:      "android",
		File:    "config_android.yaml",
		Summary: "Android projects (gradlew test/assemble)",
		Flags:   []string{"android"},
	},
	{
		ID:      "rust",
		File:    "config_rust.yaml",
		Summary: "Rust projects (cargo fmt/clippy/test)",
		Flags:   []string{"rust"},
	},
	{
		ID:      "cpp",
		File:    "config_cpp.yaml",
		Summary: "C/C++ projects (cmake/ctest)",
		Flags:   []string{"cpp", "cxx", "cplusplus"},
	},
	{
		ID:      "swift",
		File:    "config_swift.yaml",
		Summary: "Swift projects (swift test/build)",
		Flags:   []string{"swift"},
	},
	{
		ID:      "flutter",
		File:    "config_flutter.yaml",
		Summary: "Flutter projects (flutter analyze/test)",
		Flags:   []string{"flutter"},
	},
	{
		ID:      "dart",
		File:    "config_dart.yaml",
		Summary: "Dart projects (dart analyze/test)",
		Flags:   []string{"dart"},
	},
	{
		ID:      "elixir",
		File:    "config_elixir.yaml",
		Summary: "Elixir projects (mix format/test)",
		Flags:   []string{"elixir"},
	},
	{
		ID:      "deno",
		File:    "config_deno.yaml",
		Summary: "Deno projects (deno lint/format/test)",
		Flags:   []string{"deno"},
	},
	{
		ID:      "scala",
		File:    "config_scala.yaml",
		Summary: "Scala projects (sbt test)",
		Flags:   []string{"scala", "sbt"},
	},
	{
		ID:      "clojure",
		File:    "config_clojure.yaml",
		Summary: "Clojure projects (lein test)",
		Flags:   []string{"clojure", "lein", "leiningen"},
	},
	{
		ID:      "haskell",
		File:    "config_haskell.yaml",
		Summary: "Haskell projects (stack test)",
		Flags:   []string{"haskell", "stack", "cabal"},
	},
	{
		ID:      "erlang",
		File:    "config_erlang.yaml",
		Summary: "Erlang projects (rebar3 eunit)",
		Flags:   []string{"erlang", "rebar", "rebar3"},
	},
	{
		ID:      "lua",
		File:    "config_lua.yaml",
		Summary: "Lua projects (luacheck/busted)",
		Flags:   []string{"lua", "luajit"},
	},
	{
		ID:      "perl",
		File:    "config_perl.yaml",
		Summary: "Perl projects (prove)",
		Flags:   []string{"perl"},
	},
	{
		ID:      "r",
		File:    "config_r.yaml",
		Summary: "R projects (R CMD check)",
		Flags:   []string{"r", "rlang"},
	},
	{
		ID:      "terraform",
		File:    "config_terraform.yaml",
		Summary: "Terraform projects (fmt/validate)",
		Flags:   []string{"terraform", "tf"},
	},
}

func ensureDefaultPack(targetRoot string, destPath string, templateName string, force bool) error {
	if !force {
		if _, err := os.Stat(destPath); err == nil {
			return nil
		}
	}

	templateBytes, err := loadTemplateBytes(targetRoot, templateName)
	if err != nil {
		return err
	}

	return os.WriteFile(destPath, templateBytes, 0o644)
}

func loadTemplateBytes(targetRoot string, templateName string) ([]byte, error) {
	candidates := []string{
		filepath.Join(targetRoot, "assets", "templates", templateName),
	}

	if dir := strings.TrimSpace(os.Getenv("BUILDBOUNCER_TEMPLATES_DIR")); dir != "" {
		candidates = append(candidates,
			filepath.Join(dir, templateName),
			filepath.Join(dir, "templates", templateName),
		)
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "templates", templateName),
			filepath.Join(exeDir, "assets", "templates", templateName),
			filepath.Join(exeDir, "..", "share", "build-bouncer", "templates", templateName),
			filepath.Join(exeDir, "..", "libexec", "build-bouncer", "templates", templateName),
		)
	}

	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return b, nil
		}
	}

	if b, err := assettemplates.FS.ReadFile(templateName); err == nil && len(b) > 0 {
		return b, nil
	}

	return nil, errors.New("template not found: " + templateName + " (expected assets/templates or set BUILDBOUNCER_TEMPLATES_DIR)")
}

func findConfigTemplate(id string) (configTemplate, bool) {
	for _, tmpl := range configTemplates {
		if tmpl.ID == id {
			return tmpl, true
		}
	}
	return configTemplate{}, false
}

func listConfigTemplates() []configTemplate {
	return append([]configTemplate{}, configTemplates...)
}

type templateSelector struct {
	byID map[string][]*bool
}

func registerTemplateFlags(fs *flag.FlagSet) *templateSelector {
	sel := &templateSelector{byID: map[string][]*bool{}}
	for _, tmpl := range listConfigTemplates() {
		for _, name := range tmpl.Flags {
			flagName := strings.TrimPrefix(name, "--")
			var b bool
			fs.BoolVar(&b, flagName, false, "use "+tmpl.ID+" template")
			sel.byID[tmpl.ID] = append(sel.byID[tmpl.ID], &b)
		}
	}
	return sel
}

func (s *templateSelector) Selected() (string, error) {
	if s == nil {
		return "", nil
	}
	var chosen []string
	for id, flags := range s.byID {
		for _, b := range flags {
			if *b {
				chosen = append(chosen, id)
				break
			}
		}
	}
	if len(chosen) > 1 {
		sort.Strings(chosen)
		return "", fmt.Errorf("multiple templates selected: %s", strings.Join(chosen, ", "))
	}
	if len(chosen) == 1 {
		return chosen[0], nil
	}
	return "", nil
}
