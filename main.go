package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "vimi",
		Usage: "Fuzzy-find and open files in Neovim",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "preview",
				Aliases: []string{"p"},
				Usage:   "Enable preview",
			},
			&cli.BoolFlag{
				Name:    "files",
				Aliases: []string{"f"},
				Usage:   "Search directories instead of files",
			},
			&cli.IntFlag{
				Name:    "depth",
				Aliases: []string{"d"},
				Usage:   "Search depth (1 = non-recursive, 0 = unlimited)",
				Value:   0,
			},
		},
		Action: func(c *cli.Context) error {
			usePreview := c.Bool("preview")
			searchDirs := c.Bool("files")
			depth := c.Int("depth")

			paths := c.Args().Slice()
			if len(paths) == 0 {
				for _, env := range []string{"PROJECT_DIR", "WORK_DIR", "ASSET_DIR"} {
					if v := os.Getenv(env); v != "" {
						paths = append(paths, v)
					}
				}
			}
			if len(paths) == 0 {
				paths = []string{"."}
			}

			searcher, err := chooseSearcher()
			if err != nil {
				return err
			}

			itemType := "f"
			if searchDirs {
				itemType = "d"
			}

			out, err := searcher.Search(SearchOptions{ItemType: itemType, Depth: depth, Paths: paths})
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}
			if strings.TrimSpace(out) == "" {
				return nil
			}

			selected, err := fzfPick(out, buildfzfPreviewArgs(usePreview, itemType))
			if err != nil {
				return err
			}
			if selected == "" {
				return nil
			}

			if itemType == "d" {
				fmt.Println(selected)
				return nil
			}
			return openInNvim(selected)
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func chooseSearcher() (Searcher, error) {
	if commandExists("fd") {
		return &FdSearcher{}, nil
	}
	if commandExists("find") {
		return &FindSearcher{}, nil
	}
	return nil, fmt.Errorf("neither fd nor find is installed")
}

type SearchOptions struct {
	ItemType string
	Depth    int
	Paths    []string
}

type Searcher interface {
	Search(opts SearchOptions) (string, error)
	Name() string
}

type (
	FdSearcher   struct{}
	FindSearcher struct{}
)

func (s *FdSearcher) Name() string   { return "fd" }
func (s *FindSearcher) Name() string { return "find" }

func (s *FdSearcher) Search(opts SearchOptions) (string, error) {
	args := []string{"--type", opts.ItemType, "--hidden", "--follow", "--exclude", ".git"}
	if opts.Depth > 0 {
		args = append(args, "--max-depth", fmt.Sprint(opts.Depth))
	}
	for _, p := range opts.Paths {
		args = append(args, "--search-path", p)
	}
	args = append(args, ".")
	cmd := exec.Command("fd", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("fd error: %v\n%s", err, string(out))
	}
	return string(out), nil
}

func (s *FindSearcher) Search(opts SearchOptions) (string, error) {
	args := append([]string{}, opts.Paths...)
	if opts.Depth > 0 {
		args = append(args, "-maxdepth", fmt.Sprint(opts.Depth))
	}
	args = append(args, "-mindepth", "1")
	args = append(args, "-not", "-path", "*/.git/*")
	if opts.ItemType == "f" {
		args = append(args, "-type", "f")
	} else {
		args = append(args, "-type", "d")
	}
	args = append(args, "-print")
	cmd := exec.Command("find", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("find error: %v\n%s", err, string(out))
	}
	return string(out), nil
}

func fzfPick(candidates string, fzfArgs []string) (string, error) {
	cmd := exec.Command("fzf", fzfArgs...)
	cmd.Stdin = strings.NewReader(candidates)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if code := exitErr.ExitCode(); code == 130 || code == 1 {
				return "", nil
			}
		}
		return "", fmt.Errorf("fzf failed: %w", err)
	}
	return strings.TrimSpace(out.String()), nil
}

func buildfzfPreviewArgs(enable bool, itemType string) []string {
	if !enable {
		return nil
	}
	if itemType == "f" && commandExists("bat") {
		return []string{"--ansi", "--preview-window=right:45%", "--preview=bat --color=always --style=header,grid --line-range :300 {}"}
	}
	if itemType == "d" {
		return []string{"--ansi", "--preview-window=right:45%", "--preview=ls -la {}"}
	}
	return nil
}

func openInNvim(file string) error {
	cmd := exec.Command("nvim", file)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
