// Give open source Golang developers the credit they deserve, follow your legal
// obligations, and save time with `gocomply`.
//
// This tiny little 300-line program scans the Go module in the current directory
// for all direct and indirect dependencies, and attempts to download and write
// all of their license files to stdout. Progress or warnings are written to
// stderr.
//
// ## Use
//
// Install gocomply (you only need to do this once)
//
// ```
// $ go install tawesoft.co.uk/gopkg/gocomply@latest
// ```
//
// Then, go (pun not intended) to the directory of some Go module
//
// ```
// $ cd path/to/some/module
// ```
//
// Then just run `gocomply`. You probably want to redirect its output to a file,
// like so. This will overwrite that file each time.
//
// ```
// $ gocomply > 3rd-party-licenses.txt
// ```
//
// ## Important caveats
//
// Licenses of indirect dependencies will be included, regardless of whether
// they end up being used by your project or in the resulting binary. You can
// and should review and trim the output as appropriate.
//
// A human must manually check the output for compliance. Just because you have
// included the text of a license file, it does not mean you're allowed to use
// the code or that the license is open source. It does not mean that the
// author of the module that you depend on is using the license properly.
//
// The tool only checks the currently published version of a license. You might
// be using an old version that comes under a different license.
//
// The tool doesn't yet support private repos.
//
// Because `git archive` isn't widely supported (shame!) the method of
// obtaining a single license file from a git repo is something that must be
// hard-coded for each provider. The provider you use might be missing from
// this hard-coded list - if so, open an issue.
//
// The `gocomply` program also operates in a different mode where it accepts a
// list of modules to check as command-line arguments. Subtly, it is assumed that
// this is a complete list of modules and dependencies - the dependencies of
// modules provided on the command-line are NOT checked. This mode is intended for
// users who parse the output of `go list -m all` themselves.
//
package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var divider = strings.Repeat("-", 80)

const httpTimeout = 10 * time.Second

// licensesFiles are checked in this order
var licenseFiles = []string{
	"NOTICE", // apache
	"LICENSE",
	"LICENSE.txt",
	"LICENSE.md",
	"License",
	"License.txt",
	"LICENCE",
	"COPYING",
	"COPYING.txt",
	"COPYING.md",
}

func httpGet(url string) (string, error) {
	out := &bytes.Buffer{}

	client := http.Client{
		Timeout: httpTimeout,
	}

	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("http status code %d when downloading %q", resp.StatusCode, url)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}

	return out.String(), nil
}

type GoImport struct {
	ImportPrefix string
	Vcs          string
	RepoRoot     string
}

type GoSource struct {
	ImportPrefix string
	Home         string
	Directory    string
	File         string
}

// parsing HTML with regex is wrong, but this works well enough to do it anyway
var regexpGoImport = []*regexp.Regexp{
	regexp.MustCompile(`(?i)<\s*meta\s*name\s*=\s*"go-import"\s*content\s*=\s*"(?P<import_prefix>\S+)\s+(?P<vcs>\S+)\s+(?P<repo_root>\S+)"\s*/?>`),
	// source hut has the arguments the other way round
	regexp.MustCompile(`(?i)<\s*meta\s*content\s*=\s*"(?P<import_prefix>\S+)\s+(?P<vcs>\S+)\s+(?P<repo_root>\S+)"\s*name\s*=\s*"go-import"\s*/?>`),
}

func parseGoImport(data string) (GoImport, bool) {
	for _, r := range regexpGoImport {

		if !r.MatchString(data) {
			continue
		}

		matches := r.FindStringSubmatch(data)
		return GoImport{
			ImportPrefix: matches[r.SubexpIndex("import_prefix")],
			Vcs:          matches[r.SubexpIndex("vcs")],
			RepoRoot:     matches[r.SubexpIndex("repo_root")],
		}, true
	}

	return GoImport{}, false
}

var regexpGoSource = regexp.MustCompile(`(?i)<\s*meta\s*name\s*="go-source"\s*content\s*=\s*"(?P<import_prefix>\S+) (?P<home>\S+) (?P<directory>\S+) (?P<file>\S+)"\s*/?>`)

func parseGoSource(data string) (GoSource, bool) {
	r := regexpGoSource

	if !r.MatchString(data) {
		return GoSource{}, false
	}

	matches := r.FindStringSubmatch(data)
	return GoSource{
		ImportPrefix: matches[r.SubexpIndex("import_prefix")],
		Home:         matches[r.SubexpIndex("home")],
		Directory:    matches[r.SubexpIndex("directory")],
		File:         matches[r.SubexpIndex("file")],
	}, true
}

func listModules() ([]string, error) {
	stdout, err := exec.Command("go", "list", "-m", "all").Output()
	if err != nil {
		return nil, fmt.Errorf("go list error: %+v", err)
	}

	stdout = bytes.TrimSpace(stdout)
	lines := bytes.Split(stdout, []byte{'\n'})
	if len(lines) < 1 {
		return nil, fmt.Errorf("empty go list output")
	}

	// discard first line
	lines = lines[1:]

	names := make([]string, 0)
	for _, line := range lines {
		// e.g. golang.org/x/text v0.3.3
		words := bytes.SplitN(line, []byte{' '}, 2)
		if len(words) != 2 {
			return nil, fmt.Errorf("invalid go list output format (line %q)", line)
		}
		names = append(names, string(words[0]))
	}

	return names, nil
}

func stringDecoderIdentity(str string) (string, error) {
	return str, nil
}

func stringDecoderBase64(str string) (string, error) {
	bytes, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func resolveFileURL(gi GoImport, gs GoSource, file string) ([]string, func(string) (string, error), error) {
	vcs := gi.Vcs
	repoRoot := gi.RepoRoot

	if vcs != "git" {
		return nil, nil, fmt.Errorf("vcs %q not implemented", vcs)
	}

	if strings.HasPrefix(repoRoot, "https://go.googlesource.com/") {
		return []string{fmt.Sprintf("%s/+/refs/heads/master/%s?format=text", repoRoot, file)},
			stringDecoderBase64, nil
	}

	if strings.HasPrefix(repoRoot, "https://git.sr.ht/") {
		dir := strings.TrimSuffix(repoRoot, ".git")
		return []string{fmt.Sprintf("%s/blob/master/%s", dir, file)},
			stringDecoderIdentity, nil
	}

	if strings.HasPrefix(repoRoot, "https://gopkg.in/") {
		// Find correct branch including minor version.
		// The go-source meta tag for gopkg.in is the simplest place where
		// this info is exposed over HTTP, to avoid speaking git protocol.

		// e.g. gs.Directory
		// https://github.com/natefinch/lumberjack/tree/v2.1{/dir}

		user, repo, branch, ok := func() (user string, repo string, branch string, ok bool) {
			dir := strings.TrimPrefix(gs.Directory, "https://github.com/")

			parts := strings.SplitN(dir, "/", 4)
			if len(parts) != 4 {
				ok = false
				return
			}
			user = parts[0]
			repo = parts[1]
			rest := parts[3]
			idx := strings.IndexByte(rest, '{')
			if idx < 0 {
				ok = false
				return
			}

			branch = rest[0:idx]

			ok = true
			return
		}()
		if !ok {
			return nil, nil, fmt.Errorf("gopkg.in parse error")
		}

		return []string{
				fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", user, repo, branch, file),
			},
			stringDecoderIdentity, nil
	}

	if strings.HasPrefix(repoRoot, "https://github.com/") {
		dir := strings.TrimPrefix(repoRoot, "https://github.com/")
		dir = strings.TrimSuffix(dir, ".git")

		return []string{
				fmt.Sprintf("https://raw.githubusercontent.com/%s/main/%s", dir, file),
				fmt.Sprintf("https://raw.githubusercontent.com/%s/master/%s", dir, file), // historical
			},
			stringDecoderIdentity, nil
	}

	if strings.HasPrefix(repoRoot, "https://gitlab.com/") {
		dir := strings.TrimSuffix(repoRoot, ".git")

		return []string{
				fmt.Sprintf("%s/-/raw/main/%s", dir, file),
				fmt.Sprintf("%s/-/raw/master/%s", dir, file), // historical
			},
			stringDecoderIdentity, nil
	}

	return nil, nil, fmt.Errorf("repo %q not supported (please open an issue)", repoRoot)
}

func getLicense(module string, gi GoImport, gs GoSource) (string, error) {
	for _, license := range licenseFiles {
		// be a good citizen
		time.Sleep(1 * time.Second)

		licenseUrls, decoder, err := resolveFileURL(gi, gs, license)
		if err != nil {
			return "", fmt.Errorf("no known license URL for module %q: %v", module, err)
		}

		for _, licenseUrl := range licenseUrls {
			data, err := httpGet(licenseUrl)
			if err != nil {
				continue
			}

			data, err = decoder(data)
			if err != nil {
				return "", fmt.Errorf("error decoding %q: %v", licenseUrl, err)
			}

			return strings.TrimSpace(data), nil
		}
	}

	return "", fmt.Errorf("no license found for module %q", module)
}

func main() {
	err := func() error {
		var modules []string

		if len(os.Args) > 1 {
			modules = os.Args[1:]
		} else {
			var err error
			modules, err = listModules()
			if err != nil {
				return err
			}
		}

		for _, module := range modules {
			fmt.Fprintf(os.Stderr, "> %s\n", module)

			// future-proof - might take arguments in future
			if strings.HasPrefix(module, "-") {
				return fmt.Errorf("unrecognised argument %q", module)
			}

			// "golang.org is a known non-module"
			// if strings.HasPrefix(module, "golang.org") {
			//    continue
			// }

			data, err := httpGet(fmt.Sprintf("https://%s?go-get=1", module))
			if err != nil {
				// Attempt module root, for example:
				// https://github.com/go-gl/glfw/v3.3/glfw -> https://github.com/go-gl/glfw
				// https://github.com/russross/blackfriday/v2 -> https://github.com/russross/blackfriday
				parts := strings.Split(module, "/")
				if len(parts) > 3 {
					moduleroot := strings.Join(parts[:3], "/")
					data, err = httpGet(fmt.Sprintf("https://%s?go-get=1", moduleroot))
					if err != nil {
						fmt.Fprintf(os.Stderr, "error looking up module %q: %v\n", module, err)
						continue
					}
				} else {
					fmt.Fprintf(os.Stderr, "error looking up module %q: %v\n", module, err)
					continue
				}
			}

			gi, ok := parseGoImport(data)
			if !ok {
				fmt.Fprintf(os.Stderr, "unrecognised import %q (no go-import meta tags)\n", module)
				continue
			}

			gs, _ := parseGoSource(data)

			license, err := getLicense(module, gi, gs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "unable to find a license for module %q: %v\n", module, err)
				continue
			}

			fmt.Printf("%s\n\n%s\n\n%s\n\n", module, license, divider)
		}

		return nil
	}()

	if err != nil {
		panic(fmt.Sprintf("error: %v", err))
	}
}
