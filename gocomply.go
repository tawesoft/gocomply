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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jdxcode/netrc"
)

var divider = strings.Repeat("-", 80)

const httpTimeout = 10 * time.Second


// httpLicenseFiles to check, in order. For GitHub repos we have a more
// efficient way of detecting licenses. These are case sensitive if the remote
// server is case sensitive. This should be as small a list as possible.
var httpLicenseFiles = []string{
	"NOTICE", // apache, must come first
	"LICENSE",
	"LICENSE.txt",
	"LICENSE.md",
	"COPYING",
	"COPYING.txt",
	"COPYING.md",
}

// repoLicensesFiles, in order of precedence for checking in a remote
// repository. Unlike the httpLicenseFiles, we can check this case
// insensitively.
//
// This sorting is informed by the go-license-detector dataset.zip:
// `find | xargs -L1 -I{} basename "{}" | sort |  uniq -c > all.txt`
// and https://pkg.go.dev/license-policy - but we want the actual copyright
// notice and to exclude anything that's just a full copy of the GPL verbatim.
//
var repoLicenseFiles = []string{
	"NOTICE", // apache, must come first
	"NOTICE.txt", // apache, rarely
	"LICENSE",
	"LICENSE.txt",
	"LICENSE.md",
	"LICENSE.markdown",
	"LICENSE.rst",
	"LICENCE", // uncommon
	"LICENCE.txt", // uncommon
	"LICENCE.md", // uncommon
	"LICENCE.markdown", // uncommon
	"LICENCE.rst", // uncommon
	"COPYING",
	"COPYING.txt",
	"COPYRIGHT",
	"COPYRIGHT.txt",
	"MIT-LICENSE",
	"MIT-LICENSE.txt",
	"MIT-LICENCE", // uncommon
	"MIT-LICENCE.txt", // uncommon
}

type BasicAuth struct {
	Username string
	Token    string
}
var githubAuth = &BasicAuth{}

func (a BasicAuth) IsSet() bool {
	return a.Username != "" && a.Token != ""
}

func httpGet(rsc string, auth *BasicAuth) (string, error) {
	out := &bytes.Buffer{}

	client := http.Client{
		Timeout: httpTimeout,
	}

	req, err := http.NewRequest("GET", rsc, nil)
	if err != nil {
		return "", err
	}
	if (auth != nil) && auth.IsSet() {
		req.SetBasicAuth(
			url.QueryEscape(auth.Username),
			url.QueryEscape(auth.Token),
		)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("http status code %d when downloading %q", resp.StatusCode, rsc)
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
		return nil, fmt.Errorf("go list error: %+v: %s", err, err.(*exec.ExitError).Stderr)
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
		name := string(words[0])

		required, err := isRequiredModule(name)
		if err != nil { return nil, err }
		if !required { continue }

		names = append(names, name)
	}

	return names, nil
}

func isRequiredModule(name string) (bool, error) {
	// "download is split into two parts: downloading the go.mod and
	// downloading the actual code. If you have dependencies only needed for
	// tests, then they will show up in your go.mod, and go get will download
	// their go.mods, but it will not download their code."
	//
	// "This applies not just to test-only dependencies but also os-specific
	// dependencies."
	//
	// -- https://github.com/golang/go/issues/26913#issuecomment-411976222
	//
	// "The -vendor flag causes why to exclude tests of dependencies.
	//
	// "If the package or module is not
	//  referenced from the main module, the stanza will display a single
	//  parenthesized note indicating that fact."

	stdout, err := exec.Command("go", "mod", "why", "-m", "-vendor", name).Output()
	if err != nil {
		return false, fmt.Errorf("go why error: %+v: %s", err, err.(*exec.ExitError).Stderr)
	}

	lines := bytes.Split(stdout, []byte{'\n'})
	if len(lines) < 2 {
		return false, fmt.Errorf("unexpected go why output format")
	}

	// "# golang.org/x/text/encoding"
	if !bytes.Equal(bytes.TrimSpace(lines[0]), []byte("# " + name)) {
		return false, fmt.Errorf("unexpected go why output format")
	}

	// "(main module does not need package golang.org/x/text/encoding)"
	line := bytes.TrimSpace(lines[1])
	if (len(line) > 2) && line[0] == '(' && line[len(line)-1] == ')' {
		return false, nil
	}

	// any other result means its used
	return true, nil
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

	// try API
	if gi.Vcs == "git" && strings.HasPrefix(gi.RepoRoot, "https://github.com/") && githubAuth.IsSet() {
		// TODO check rate limits

		license, missing, err := func() (string, bool, error) {
			// rate limit is 5000 hour once authenticated - as low as 50/hour when anonymous!
			// TODO we could reduce this timeout when rate is high
			time.Sleep(2 * 1230 * time.Millisecond)

			// TODO if we refactor resolveFileURL to make it more general purpose
			//   then this could work for gopkg.in too

			// TODO make this a method on gi to stop repeating this
			dir := strings.TrimPrefix(gi.RepoRoot, "https://github.com/")
			dir = strings.TrimSuffix(dir, ".git")

			data, err := httpGet(fmt.Sprintf("https://api.github.com/repos/%s/git/trees/HEAD", dir), githubAuth)
			if err != nil {
				return "", false, fmt.Errorf("trouble getting listing for %s: %v", gi.RepoRoot, err)
			}

			type APITree struct {
				Path string
				Type string // we want "blob"
				Url  string
			}

			type APIResponse struct {
				Tree []APITree
			}

			type APIBlob struct {
				Content string
				Encoding string
			}

			var response APIResponse
			err = json.Unmarshal([]byte(data), &response)
			if err != nil {
				return "", false, fmt.Errorf("json decode error: %v", err)
			}

			for _, t := range response.Tree {
				if t.Type != "blob" { continue }
				for _, name := range repoLicenseFiles {
					if !strings.EqualFold(t.Path, name) { continue }

					data, err := httpGet(t.Url, githubAuth)
					if err != nil {
						return "", false, fmt.Errorf("trouble getting blob for %s: %v", gi.RepoRoot, err)
					}

					var blob APIBlob
					err = json.Unmarshal([]byte(data), &blob)
					if err != nil {
						return "", false, fmt.Errorf("json decode error: %v", err)
					}

					if strings.EqualFold(blob.Encoding, "utf-8") {
						return strings.TrimSpace(blob.Content), false, nil
					} else if strings.EqualFold(blob.Encoding, "base64") {
						raw, err := base64.StdEncoding.DecodeString(blob.Content)
						if err != nil {
							return "", false, fmt.Errorf("base64 decode error: %v", err)
						}
						return strings.TrimSpace(string(raw)), false, nil
					} else {
						return "", false, fmt.Errorf("unknown encoding type %q", blob.Encoding)
					}
				}
			}

			return "", true, fmt.Errorf("no license found")
		}()

		if err == nil {
			return license, nil
		} else {
			err = fmt.Errorf("api.github.com error: %s", err)

			if missing {
				return "", err
			} else {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				// proceed to fallback
			}
		}
	}

	return tryGetLicense(module, gi, gs, httpLicenseFiles)
}

func tryGetLicense(module string, gi GoImport, gs GoSource, files []string) (string, error) {
	for _, license := range files {
		// be a good citizen
		time.Sleep(1 * time.Second)

		licenseUrls, decoder, err := resolveFileURL(gi, gs, license)
		if err != nil {
			return "", fmt.Errorf("no known license URL for module %q: %v", module, err)
		}

		for _, licenseUrl := range licenseUrls {
			data, err := httpGet(licenseUrl, nil)
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

func lookup(module string) (gi GoImport, gs GoSource, err error) {
	var data string
	var ok bool

	data, err = httpGet(fmt.Sprintf("https://%s?go-get=1", module), nil)
	if err != nil {
		// Attempt module root, for example:
		// https://github.com/go-gl/glfw/v3.3/glfw -> https://github.com/go-gl/glfw
		// https://github.com/russross/blackfriday/v2 -> https://github.com/russross/blackfriday
		parts := strings.Split(module, "/")
		if len(parts) > 3 {
			moduleroot := strings.Join(parts[:3], "/")
			data, err = httpGet(fmt.Sprintf("https://%s?go-get=1", moduleroot), nil)
		}

		if err != nil {
			// Assume its a private repo
			// TODO should check this against go env GOPRIVATE
			// and should do that before attempting module root
			gi = GoImport{
				ImportPrefix: module,
				Vcs:          "git",
				RepoRoot:     fmt.Sprintf("https://%s.git", module),
			}
			return gi, gs, nil
		}
	}

	gi, ok = parseGoImport(data)
	if !ok {
		err = fmt.Errorf("unrecognised import %q (no go-import meta tags)", module)
		return
	}

	gs, _ = parseGoSource(data)

	return gi, gs, nil
}

func parseNetrc() error {
	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("user lookup error: %v", err)
	}

	netrcPath := os.Getenv("NETRC")
	if netrcPath == "" {
		netrcPath = filepath.Join(usr.HomeDir, ".netrc")
	}

	n, err := netrc.Parse(netrcPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) { return nil }
		return fmt.Errorf(".netrc parse error: %v", err)
	}

	github := n.Machine("github.com")
	if github != nil {
		githubAuth = &BasicAuth{
			Username: github.Get("login"),
			Token:    github.Get("password"),
		}
	}

	return nil
}

func main() {

	parseNetrc()

	if githubAuth == nil || !githubAuth.IsSet() {
		fmt.Fprintf(os.Stderr, "warning: no credentials set for GitHub API\n -- gocomply may be slower and less accurate\n")
	}

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

		// the standard library
		modules = append(modules, "github.com/golang/go")

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

			gi, gs, err := lookup(module)
			if err != nil {
				fmt.Fprintf(os.Stderr, "unable to lookup module %q: %v\n", module, err)
				continue
			}

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
