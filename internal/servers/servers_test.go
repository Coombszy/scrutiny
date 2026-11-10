package servers_test

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/installer"
	"github.com/Coombszy/scrutiny/internal/servers"
)

var registry = []installer.Package{
	{Name: "gopls", Languages: []string{"Go"}, Version: "v0.23.0", Source: "golang", Requires: []string{"go"}},
	{Name: "typescript-language-server", Languages: []string{"TypeScript", "JavaScript"}, Version: "6.0.3", Source: "npm", Requires: []string{"node", "npm"}},
	{Name: "rust-analyzer", Languages: []string{"Rust"}, Version: "2026-11-09", Source: "github"},
	{Name: "jdtls", Languages: []string{"Java"}, Source: "generic", Unsupported: "installs from generic"},
}

func newService(inst *installer.Fake, onPath ...string) *servers.Service {
	return servers.New(inst, func(name string) (string, error) {
		if slices.Contains(onPath, name) {
			return "/usr/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	})
}

func TestCommandsLaunchInstalledServersForTheirLanguages(t *testing.T) {
	inst := installer.NewFake(registry...)
	inst.Have["typescript-language-server"] = installer.Installed{
		Name: "typescript-language-server", Languages: []string{"TypeScript", "JavaScript"},
		Bin: map[string]string{"typescript-language-server": "/servers/bin/typescript-language-server"},
	}
	svc := newService(inst, "gopls")

	for lang, want := range map[string][]string{
		"typescript": {"/servers/bin/typescript-language-server", "--stdio"},
		"javascript": {"/servers/bin/typescript-language-server", "--stdio"},
		"go":         {"/usr/bin/gopls"}, // not installed, but on $PATH
		"rust":       nil,
	} {
		if got := svc.Command(lang); !slices.Equal(got, want) {
			t.Errorf("Command(%q) = %v, want %v", lang, got, want)
		}
	}
}

func TestListFiltersAndPutsInstalledFirst(t *testing.T) {
	inst := installer.NewFake(registry...)
	inst.Absent["node"] = true
	inst.Have["rust-analyzer"] = installer.Installed{Name: "rust-analyzer", Version: "2026-11-09", Languages: []string{"Rust"}}
	svc := newService(inst)

	all, err := svc.List(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if all[0].Name != "rust-analyzer" || all[0].Installed == nil {
		t.Errorf("first = %+v, want installed rust-analyzer", all[0])
	}
	ts, _ := svc.List(context.Background(), "javascr")
	if len(ts) != 1 || ts[0].Name != "typescript-language-server" || !slices.Equal(ts[0].Missing, []string{"node"}) {
		t.Errorf("List(javascr) = %+v", ts)
	}
}

func TestInstallRunsInTheBackground(t *testing.T) {
	inst := installer.NewFake(registry...)
	inst.Block = make(chan struct{})
	svc := newService(inst)

	svc.Install("gopls")
	if st := status(t, svc, "gopls"); st.Job == nil || st.Job.State != servers.Installing {
		t.Fatalf("status while installing = %+v", st)
	}
	close(inst.Block)
	waitFor(t, func() bool { st := status(t, svc, "gopls"); return st.Installed != nil && st.Job == nil })
}

func TestFailedInstallKeepsItsError(t *testing.T) {
	inst := installer.NewFake(registry...)
	inst.Fail = errors.New("npm exploded")
	svc := newService(inst)

	svc.Install("typescript-language-server")
	waitFor(t, func() bool {
		st := status(t, svc, "typescript-language-server")
		return st.Job != nil && st.Job.State == servers.Failed && st.Job.Err == "npm exploded"
	})
}

func TestRemove(t *testing.T) {
	inst := installer.NewFake(registry...)
	inst.Have["gopls"] = installer.Installed{Name: "gopls"}
	svc := newService(inst)
	if err := svc.Remove("gopls"); err != nil {
		t.Fatal(err)
	}
	if st := status(t, svc, "gopls"); st.Installed != nil {
		t.Errorf("still installed: %+v", st)
	}
}

func status(t *testing.T, svc *servers.Service, name string) servers.Status {
	t.Helper()
	st, err := svc.Status(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
