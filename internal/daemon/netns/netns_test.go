package netns

import (
	"strings"
	"testing"
)

type fakeNetlink struct {
	eth0      link
	eth0Err   error
	hostByIdx map[int]link
}

func (f fakeNetlink) podEth0(_ string) (link, error) {
	if f.eth0Err != nil {
		return link{}, f.eth0Err
	}
	return f.eth0, nil
}

func (f fakeNetlink) hostLinkByIndex(index int) (link, error) {
	l, ok := f.hostByIdx[index]
	if !ok {
		return link{}, &notFoundError{index: index}
	}
	return l, nil
}

type notFoundError struct{ index int }

func (e *notFoundError) Error() string { return "link not found" }

func TestMatchHostVeth(t *testing.T) {
	tests := []struct {
		name         string
		nl           fakeNetlink
		wantHostIdx  int
		wantPodIdx   int
		wantErrMatch string
	}{
		{
			name: "valid veth pair resolves host-side ifindex",
			nl: fakeNetlink{
				eth0: link{index: 7, name: "eth0", peerIdx: 42},
				hostByIdx: map[int]link{
					42: {index: 42, name: "veth1a2b3c", peerIdx: 7},
				},
			},
			wantHostIdx: 42,
			wantPodIdx:  7,
		},
		{
			name: "pod eth0 without veth peer is rejected",
			nl: fakeNetlink{
				eth0: link{index: 7, name: "eth0", peerIdx: 0},
			},
			wantErrMatch: "no veth peer",
		},
		{
			name: "host peer ifindex not present in host netns",
			nl: fakeNetlink{
				eth0:      link{index: 7, name: "eth0", peerIdx: 99},
				hostByIdx: map[int]link{},
			},
			wantErrMatch: "not found in host netns",
		},
		{
			name: "non-reciprocal pairing is rejected",
			nl: fakeNetlink{
				eth0: link{index: 7, name: "eth0", peerIdx: 42},
				hostByIdx: map[int]link{
					42: {index: 42, name: "veth1a2b3c", peerIdx: 5},
				},
			},
			wantErrMatch: "pairing mismatch",
		},
		{
			name: "pod eth0 read failure propagates",
			nl: fakeNetlink{
				eth0Err: &notFoundError{index: 7},
			},
			wantErrMatch: "read pod eth0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hostLink, podIdx, err := matchHostVeth(tc.nl, "/proc/1234/root/sys/class/net")
			if tc.wantErrMatch != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrMatch)
				}
				if !strings.Contains(err.Error(), tc.wantErrMatch) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErrMatch)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hostLink.index != tc.wantHostIdx {
				t.Fatalf("hostVethIfindex = %d, want %d", hostLink.index, tc.wantHostIdx)
			}
			if podIdx != tc.wantPodIdx {
				t.Fatalf("podEth0Ifindex = %d, want %d", podIdx, tc.wantPodIdx)
			}
		})
	}
}

func TestParseCgroupV2Path(t *testing.T) {
	tests := []struct {
		name         string
		contents     string
		want         string
		wantErrMatch string
	}{
		{
			name:     "unified hierarchy line",
			contents: "0::/kubepods.slice/kubepods-besteffort.slice/pod1234/abc",
			want:     "/kubepods.slice/kubepods-besteffort.slice/pod1234/abc",
		},
		{
			name:     "unified line among v1 controllers",
			contents: "12:pids:/foo\n11:memory:/foo\n0::/kubepods/pod999/cid",
			want:     "/kubepods/pod999/cid",
		},
		{
			name:         "v1-only is rejected",
			contents:     "12:pids:/foo\n11:memory:/foo",
			wantErrMatch: "cgroup v2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCgroupV2Path(tc.contents)
			if tc.wantErrMatch != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrMatch) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErrMatch, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParsePIDFromInfo(t *testing.T) {
	tests := []struct {
		name         string
		info         map[string]string
		want         int
		wantErrMatch string
	}{
		{
			name: "containerd info json blob",
			info: map[string]string{"info": `{"pid":4242,"sandboxID":"abc"}`},
			want: 4242,
		},
		{
			name: "flat pid key fallback",
			info: map[string]string{"pid": "777"},
			want: 777,
		},
		{
			name:         "missing pid",
			info:         map[string]string{"other": "x"},
			wantErrMatch: "missing",
		},
		{
			name:         "non-positive pid rejected",
			info:         map[string]string{"info": `{"pid":0}`},
			wantErrMatch: "non-positive",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePIDFromInfo(tc.info)
			if tc.wantErrMatch != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrMatch) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErrMatch, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestParseIfindexFile(t *testing.T) {
	got, err := parseIfindexFile("  42\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
	if _, err := parseIfindexFile("notanumber"); err == nil {
		t.Fatal("expected error for non-numeric ifindex")
	}
}
