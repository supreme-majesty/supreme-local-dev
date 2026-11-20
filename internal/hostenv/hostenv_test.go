package hostenv

import (
	"bufio"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/mount"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

func containerEnv() *Env {
	return Container("abc123", []Mount{
		{Type: "bind", Source: "/var/run/docker.sock", Destination: "/var/run/docker.sock"},
		{Type: "volume", Name: "sld-data", Destination: "/data"},
		{Type: "bind", Source: "/home/dev/Projects", Destination: "/workspace"},
		{Type: "bind", Source: "/home/dev/oss", Destination: "/workspace/oss"},
		{Type: "bind", Source: `C:\Users\dev\Code`, Destination: "/win"},
	}, "/data")
}

func TestMountFor_HostModePassesPathThrough(t *testing.T) {
	env := Host(t.TempDir())
	m, err := env.MountFor("/home/dev/app", "/app", true)
	if err != nil {
		t.Fatal(err)
	}
	want := reconcile.Mount{Type: reconcile.MountTypeBind, Source: "/home/dev/app", Target: "/app", ReadOnly: true}
	if m != want {
		t.Errorf("got %+v, want %+v", m, want)
	}
}

func TestMountFor_ContainerMode(t *testing.T) {
	env := containerEnv()
	tests := []struct {
		name  string
		local string
		want  reconcile.Mount
	}{
		{
			name:  "bind mount subdirectory is translated to the host path",
			local: "/workspace/shop",
			want:  reconcile.Mount{Type: "bind", Source: "/home/dev/Projects/shop", Target: "/app"},
		},
		{
			name:  "longest matching mount wins",
			local: "/workspace/oss/lib",
			want:  reconcile.Mount{Type: "bind", Source: "/home/dev/oss/lib", Target: "/app"},
		},
		{
			name:  "mount root itself",
			local: "/workspace",
			want:  reconcile.Mount{Type: "bind", Source: "/home/dev/Projects", Target: "/app"},
		},
		{
			name:  "named volume becomes a volume subpath mount",
			local: "/data/certs",
			want:  reconcile.Mount{Type: "volume", Source: "sld-data", Target: "/app", Subpath: "certs"},
		},
		{
			name:  "windows host path keeps backslashes",
			local: "/win/site/public",
			want:  reconcile.Mount{Type: "bind", Source: `C:\Users\dev\Code\site\public`, Target: "/app"},
		},
		{
			name:  "unclean path is cleaned before matching",
			local: "/workspace/shop/../blog/",
			want:  reconcile.Mount{Type: "bind", Source: "/home/dev/Projects/blog", Target: "/app"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := env.MountFor(tt.local, "/app", false)
			if err != nil {
				t.Fatalf("MountFor(%q): %v", tt.local, err)
			}
			if got != tt.want {
				t.Errorf("MountFor(%q) = %+v, want %+v", tt.local, got, tt.want)
			}
		})
	}
}

func TestMountFor_RejectsUnmountedPaths(t *testing.T) {
	env := containerEnv()
	for _, p := range []string{"/home/sld/project", "/workspacex/app", "/"} {
		if _, err := env.MountFor(p, "/app", false); !errors.Is(err, ErrNotMounted) {
			t.Errorf("MountFor(%q) error = %v, want ErrNotMounted", p, err)
		}
	}
}

func TestMountFor_IgnoresRootMount(t *testing.T) {
	env := Container("x", []Mount{{Type: "bind", Source: "/", Destination: "/"}}, "/data")
	if _, err := env.MountFor("/etc", "/x", true); !errors.Is(err, ErrNotMounted) {
		t.Fatalf("a root mount must not be used for translation, got %v", err)
	}
}

func TestHostPath(t *testing.T) {
	env := containerEnv()
	got, err := env.HostPath("/workspace/shop")
	if err != nil || got != "/home/dev/Projects/shop" {
		t.Errorf("HostPath = %q, %v", got, err)
	}
	if _, err := env.HostPath("/data/certs"); !errors.Is(err, ErrNotHostPath) {
		t.Errorf("volume path should return ErrNotHostPath, got %v", err)
	}
}

func TestProjectIdentity_UsesHostPathInContainer(t *testing.T) {
	env := containerEnv()
	inContainer, err := env.ProjectIdentity("/workspace/shop")
	if err != nil {
		t.Fatal(err)
	}
	onHost, err := project.ComputeIdentityFromHostPath("/home/dev/Projects/shop")
	if err != nil {
		t.Fatal(err)
	}
	if inContainer.ID != onHost.ID {
		t.Errorf("container ID %s != host ID %s", inContainer.ID, onHost.ID)
	}

	// The same folder mounted at a different container path keeps its identity.
	moved := Container("abc123", []Mount{{Type: "bind", Source: "/home/dev/Projects", Destination: "/code"}}, "/data")
	again, err := moved.ProjectIdentity("/code/shop")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != inContainer.ID {
		t.Errorf("identity changed with mount point: %s vs %s", again.ID, inContainer.ID)
	}
}

func TestPublishPort(t *testing.T) {
	t.Setenv(envBindHost, "")
	if got := Host(t.TempDir()).PublishPort(8080, "80"); got != "127.0.0.1:8080:80" {
		t.Errorf("default PublishPort = %q", got)
	}
	t.Setenv(envBindHost, "0.0.0.0")
	if got := Host(t.TempDir()).PublishPort(443, "443"); got != "0.0.0.0:443:443" {
		t.Errorf("overridden PublishPort = %q", got)
	}
}

type fakeInspector struct {
	id   string
	info types.ContainerJSON
	err  error
}

func (f *fakeInspector) InspectContainer(_ context.Context, id string) (types.ContainerJSON, error) {
	f.id = id
	return f.info, f.err
}

func TestDetect(t *testing.T) {
	t.Run("host mode", func(t *testing.T) {
		t.Setenv(envInContainer, "0")
		env, err := Detect(context.Background(), &fakeInspector{})
		if err != nil || env.InContainer() {
			t.Fatalf("expected host mode, got inContainer=%v err=%v", env.InContainer(), err)
		}
	})

	t.Run("container mode reads own mounts", func(t *testing.T) {
		t.Setenv(envInContainer, "1")
		t.Setenv(envContainerID, "self-id")
		t.Setenv(envStateDir, "/data")
		fi := &fakeInspector{info: types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{ID: "full-self-id"},
			Mounts: []types.MountPoint{
				{Type: mount.TypeBind, Source: "/srv/code", Destination: "/workspace"},
				{Type: mount.TypeVolume, Name: "sld-data", Destination: "/data"},
			},
		}}
		env, err := Detect(context.Background(), fi)
		if err != nil {
			t.Fatal(err)
		}
		if fi.id != "self-id" || env.ContainerID() != "full-self-id" || !env.InContainer() {
			t.Fatalf("unexpected env: inspected=%q id=%q in=%v", fi.id, env.ContainerID(), env.InContainer())
		}
		m, err := env.MountFor("/workspace/a", "/app", false)
		if err != nil || m.Source != "/srv/code/a" {
			t.Errorf("MountFor after Detect = %+v, %v", m, err)
		}
	})

	t.Run("inspect failure is reported", func(t *testing.T) {
		t.Setenv(envInContainer, "1")
		t.Setenv(envContainerID, "self-id")
		if _, err := Detect(context.Background(), &fakeInspector{err: errors.New("no socket")}); err == nil {
			t.Fatal("expected an error when self-inspection fails")
		}
	})
}

func TestContainerIDFromMountinfo(t *testing.T) {
	id := strings.Repeat("ab", 32)
	info := "123 98 0:42 / / rw - overlay overlay rw\n" +
		"456 123 8:1 /var/lib/docker/containers/" + id + "/hostname /etc/hostname rw - ext4 /dev/sda1 rw\n"
	if got := containerIDFromMountinfo(bufio.NewScanner(strings.NewReader(info))); got != id {
		t.Errorf("got %q, want %q", got, id)
	}
	if got := containerIDFromMountinfo(bufio.NewScanner(strings.NewReader("1 2 0:1 / / rw - ext4 x rw\n"))); got != "" {
		t.Errorf("expected no ID, got %q", got)
	}
}

func TestRouterPortsAndSiteURL(t *testing.T) {
	t.Setenv(envHTTPPort, "")
	t.Setenv(envHTTPSPort, "")
	env := Host(t.TempDir())
	if env.HTTPPort() != 80 || env.HTTPSPort() != 443 {
		t.Fatalf("default ports = %d/%d", env.HTTPPort(), env.HTTPSPort())
	}
	if got := env.SiteURL("shop.localhost", true); got != "https://shop.localhost" {
		t.Errorf("SiteURL default https = %q", got)
	}

	t.Setenv(envHTTPPort, "18080")
	t.Setenv(envHTTPSPort, "18443")
	if got := env.SiteURL("shop.localhost", false); got != "http://shop.localhost:18080" {
		t.Errorf("SiteURL http = %q", got)
	}
	if got := env.SiteURL("shop.localhost", true); got != "https://shop.localhost:18443" {
		t.Errorf("SiteURL https = %q", got)
	}

	t.Setenv(envHTTPPort, "not-a-port")
	if env.HTTPPort() != 80 {
		t.Errorf("invalid SLD_HTTP_PORT should fall back to 80, got %d", env.HTTPPort())
	}
}

func TestMirrorMount(t *testing.T) {
	env := containerEnv()
	m, target, err := env.MirrorMount("/win/site", false)
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != `C:\Users\dev\Code\site` || target != "/run/desktop/mnt/host/c/Users/dev/Code/site" || m.Target != target {
		t.Errorf("windows mirror: source=%q target=%q", m.Source, target)
	}
	m, target, err = env.MirrorMount("/workspace/shop", true)
	if err != nil || target != "/home/dev/Projects/shop" || m.Source != "/home/dev/Projects/shop" || !m.ReadOnly {
		t.Errorf("linux mirror: %+v %q %v", m, target, err)
	}
	if _, _, err := env.MirrorMount("/data/x", false); err == nil {
		t.Error("volume-backed paths cannot be mirrored")
	}
}
