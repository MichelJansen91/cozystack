package main

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	mount "k8s.io/mount-utils"
)

// A path that does not exist, so the fake mounter's symlink resolution keeps
// it as is and matches it against its mount table.
const tempMountPath = "/nonexistent/nfs-init-pvc-1-123"

// forceFakeMounter adds mount.MounterForceUnmounter to the fake, as the real
// Linux mounter implements it.
type forceFakeMounter struct {
	*mount.FakeMounter
	forceErr      error
	forcedTargets []string
	forcedTimeout time.Duration
}

func (f *forceFakeMounter) UnmountWithForce(target string, timeout time.Duration) error {
	f.forcedTargets = append(f.forcedTargets, target)
	f.forcedTimeout = timeout
	return f.forceErr
}

func mountedFake(unmountErr error) *mount.FakeMounter {
	m := mount.NewFakeMounter([]mount.MountPoint{{Path: tempMountPath, Type: "nfs"}})
	if unmountErr != nil {
		m.UnmountFunc = func(string) error { return unmountErr }
	}
	return m
}

func TestUnmountTempMountUsesForceUnmounter(t *testing.T) {
	m := &forceFakeMounter{FakeMounter: mountedFake(errors.New("plain Unmount called"))}

	if err := unmountTempMount(m, tempMountPath); err != nil {
		t.Fatalf("unmountTempMount: %v", err)
	}
	if len(m.forcedTargets) != 1 || m.forcedTargets[0] != tempMountPath {
		t.Fatalf("UnmountWithForce targets = %v, want [%s]", m.forcedTargets, tempMountPath)
	}
	if m.forcedTimeout <= 0 || m.forcedTimeout >= kubeletCSIOperationTimeout {
		t.Errorf("UnmountWithForce timeout = %v, want within (0, %v)", m.forcedTimeout, kubeletCSIOperationTimeout)
	}
}

// main wires mount.New(""); without this the force path above is never taken
// in production and the fallback is silent.
func TestLinuxMounterIsForceUnmounter(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("mount.New returns the unsupported mounter off Linux")
	}
	if _, ok := mount.New("").(mount.MounterForceUnmounter); !ok {
		t.Fatal("mount.New(\"\") does not implement mount.MounterForceUnmounter")
	}
}

func TestUnmountTempMountReportsForceFailure(t *testing.T) {
	m := &forceFakeMounter{FakeMounter: mountedFake(nil), forceErr: errors.New("device busy")}

	if err := unmountTempMount(m, tempMountPath); err == nil {
		t.Fatal("unmountTempMount succeeded although UnmountWithForce failed")
	}
}

func TestUnmountTempMountFallsBackToPlainUnmount(t *testing.T) {
	m := mountedFake(nil)
	if err := unmountTempMount(m, tempMountPath); err != nil {
		t.Fatalf("unmountTempMount: %v", err)
	}
	if mps, _ := m.List(); len(mps) != 0 {
		t.Errorf("mount points after unmount = %v, want none", mps)
	}

	if err := unmountTempMount(mountedFake(errors.New("device busy")), tempMountPath); err == nil {
		t.Error("unmountTempMount succeeded although Unmount failed")
	}
}

func TestNodePublishVolumeNFSUnmountsTempMountWithForce(t *testing.T) {
	tmpRoot := t.TempDir()
	t.Setenv("TMPDIR", tmpRoot)

	m := &forceFakeMounter{FakeMounter: mount.NewFakeMounter(nil)}
	w := &WrappedNodeService{mounter: m}
	req := &csi.NodePublishVolumeRequest{
		VolumeId:       "pvc-1",
		TargetPath:     filepath.Join(t.TempDir(), "target"),
		PublishContext: map[string]string{nfsExportKey: "nfs://192.0.2.10:2049/export"},
	}
	if _, err := w.NodePublishVolume(context.Background(), req); err != nil {
		t.Fatalf("NodePublishVolume: %v", err)
	}

	if len(m.forcedTargets) != 1 || !strings.HasPrefix(filepath.Base(m.forcedTargets[0]), "nfs-init-pvc-1-") {
		t.Fatalf("UnmountWithForce targets = %v, want the one nfs-init-pvc-1-* temp mount", m.forcedTargets)
	}
}
