package settings

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildArchive returns a gzip+tar archive containing the given name->content entries.
func buildArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeTemp(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// --- ValidateArchive ------------------------------------------------------------------

func TestValidateAcceptsIntactArchive(t *testing.T) {
	dir := t.TempDir()
	p := writeTemp(t, dir, "good.tar.gz", buildArchive(t, map[string]string{"offline/46/-124/data": "hello"}))
	if err := ValidateArchive(p); err != nil {
		t.Fatalf("intact archive rejected: %v", err)
	}
}

// This is THE case that broke the device: the file looks like a gzip, has a valid header, and only
// reveals itself as short when the stream is read to its CRC/length trailer.
func TestValidateRejectsTruncatedArchive(t *testing.T) {
	dir := t.TempDir()
	full := buildArchive(t, map[string]string{"offline/46/-124/data": strings.Repeat("x", 40000)})
	// 99% matters most: it is only a couple of bytes short, which is what a connection dying at the
	// very end looks like. That case slipped through until ValidateArchive drained the gzip trailer.
	for _, frac := range []int{10, 50, 90, 99} {
		cut := len(full) * frac / 100
		p := writeTemp(t, dir, "trunc.tar.gz", full[:cut])
		if err := ValidateArchive(p); err == nil {
			t.Fatalf("truncated archive (%d%% of %d bytes) passed validation", frac, len(full))
		}
	}
}

func TestValidateRejectsGarbageAndEmpty(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string][]byte{
		"garbage.tar.gz": []byte("this is not a gzip stream at all"),
		"empty.tar.gz":   {},
	} {
		if err := ValidateArchive(writeTemp(t, dir, name, data)); err == nil {
			t.Fatalf("%s passed validation", name)
		}
	}
}

func TestValidateRejectsArchiveWithNoFiles(t *testing.T) {
	dir := t.TempDir()
	p := writeTemp(t, dir, "nofiles.tar.gz", buildArchive(t, map[string]string{}))
	if err := ValidateArchive(p); err == nil {
		t.Fatal("empty-but-well-formed archive passed validation")
	}
}

func TestValidateRejectsMissingFile(t *testing.T) {
	if err := ValidateArchive(filepath.Join(t.TempDir(), "nope.tar.gz")); err == nil {
		t.Fatal("missing file passed validation")
	}
}

// --- DownloadFile ---------------------------------------------------------------------

func TestDownloadWritesCompleteFileAndLeavesNoPart(t *testing.T) {
	body := []byte("complete payload")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	if err := DownloadFile(srv.URL, dest); err != nil {
		t.Fatalf("download failed: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("content mismatch: %q vs %q (%v)", got, body, err)
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part file left behind after a successful download")
	}
}

// The server declares a Content-Length it does not honour -- the shape of a connection that drops
// mid-body. Without the length check this lands as a short file that reads as a clean EOF.
func TestDownloadRejectsShortBodyAgainstContentLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("only a few bytes"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "short.bin")
	if err := DownloadFile(srv.URL, dest); err == nil {
		t.Fatal("short body accepted despite Content-Length mismatch")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("a truncated download was published to the destination path")
	}
}

func TestDownloadDoesNotPublishOnHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "err.bin")
	if err := DownloadFile(srv.URL, dest); err == nil {
		t.Fatal("HTTP 500 accepted")
	}
	for _, p := range []string{dest, dest + ".part"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s should not exist after a failed download", p)
		}
	}
}

// A leftover .part from a previous interrupted attempt must not contaminate the next one.
func TestDownloadOverwritesStalePartFile(t *testing.T) {
	body := []byte("fresh")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")
	writeTemp(t, dir, "out.bin.part", []byte("stale garbage from a killed run"))

	if err := DownloadFile(srv.URL, dest); err != nil {
		t.Fatalf("download failed: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, body) {
		t.Fatalf("stale .part contaminated the result: %q", got)
	}
}

// The POINT of the .part indirection is crash safety: while bytes are in flight, the destination
// path must not exist, so a process killed mid-transfer leaves no file that a later run could
// mistake for a complete tile. Asserted from inside the server handler, which runs while
// DownloadFile is still copying.
func TestDestinationDoesNotExistWhileTransferIsInFlight(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "inflight.bin")

	var midDestExists, midPartExists bool
	checked := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("a"), 4096))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// The client is now mid-copy; look at what is on disk.
		_, destErr := os.Stat(dest)
		_, partErr := os.Stat(dest + ".part")
		midDestExists = destErr == nil
		midPartExists = partErr == nil
		close(checked)
		w.Write(bytes.Repeat([]byte("b"), 4096))
	}))
	defer srv.Close()

	if err := DownloadFile(srv.URL, dest); err != nil {
		t.Fatalf("download failed: %v", err)
	}
	<-checked
	if midDestExists {
		t.Fatal("destination path existed mid-transfer: a crash here would leave a truncated tile")
	}
	if !midPartExists {
		t.Fatal("no .part file during transfer -- bytes are not being staged")
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("destination missing after a successful download: %v", err)
	}
}

// --- writeFileAtomic ------------------------------------------------------------------

// The original extractor opened targets with O_CREATE|O_RDWR and no O_TRUNC. Writing a SHORTER file
// over a longer one left the old tail attached, producing a corrupt hybrid that SURVIVED
// re-downloading -- which is why repeated repair attempts could not fix a bad tile.
//
// NOTE on what actually fixes this: the temp-file + rename does. Writing into a freshly-created
// .part and renaming replaces the target wholesale, so length can never carry over regardless of
// the open flags. O_TRUNC is belt-and-braces (it matters only if the .part itself survives), and
// removing it does NOT make this test fail -- verified by mutation. The test is kept because it
// pins the observable property (no stale tail) rather than the mechanism.
func TestExtractReplacesLongerFileWholesale(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tile")
	old := strings.Repeat("O", 500)
	if err := os.WriteFile(target, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	newContent := "short new content"
	if err := writeFileAtomic(target, 0o644, strings.NewReader(newContent)); err != nil {
		t.Fatalf("atomic write failed: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != newContent {
		t.Fatalf("stale tail survived the overwrite: got %d bytes %q, want %d", len(got), got, len(newContent))
	}
}

func TestExtractLeavesNoPartOnSuccess(t *testing.T) {
	target := filepath.Join(t.TempDir(), "tile")
	if err := writeFileAtomic(target, 0o644, strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part left behind after a successful extract")
	}
}

// --- safeJoin: tar-slip ---------------------------------------------------------------

func TestSafeJoinRejectsEscapingEntries(t *testing.T) {
	base := "/data/media/0/osm"
	// NB: an ABSOLUTE entry name like "/etc/shadow" is NOT in this list. filepath.Join neutralizes
	// it into base+"/etc/shadow", which stays inside the destination -- it is ugly but not an
	// escape. Only ".." traversal actually gets out.
	for _, name := range []string{
		"../../../etc/passwd",
		"offline/../../../../tmp/evil",
		"../offline/46/-124/data",
	} {
		if _, err := safeJoin(base, name); err == nil {
			t.Fatalf("entry %q was allowed to escape the destination", name)
		}
	}
}

func TestSafeJoinAllowsNormalEntries(t *testing.T) {
	base := "/data/media/0/osm"
	for _, name := range []string{"offline/46/-124/data", "offline/44/-122/bounds", "/etc/shadow"} {
		got, err := safeJoin(base, name)
		if err != nil {
			t.Fatalf("legitimate entry %q rejected: %v", name, err)
		}
		if !strings.HasPrefix(got, base) {
			t.Fatalf("unexpected target %q", got)
		}
	}
}

// --- end to end -----------------------------------------------------------------------

// A truncated archive must leave existing good tiles untouched: validation runs before extraction.
func TestTruncatedArchiveNeverTouchesLiveTiles(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "existing_tile")
	good := strings.Repeat("G", 300)
	if err := os.WriteFile(live, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}

	full := buildArchive(t, map[string]string{"existing_tile": "replacement"})
	p := writeTemp(t, dir, "bad.tar.gz", full[:len(full)*60/100])

	if err := ValidateArchive(p); err == nil {
		t.Fatal("truncated archive passed validation")
	}
	// extraction is gated on validation, so the live tile is still the original
	got, _ := os.ReadFile(live)
	if string(got) != good {
		t.Fatal("a live tile was modified by a download that failed validation")
	}
}

var _ = io.Discard
