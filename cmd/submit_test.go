package cmd

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/exercism/cli/config"
	"github.com/exercism/cli/workspace"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestSubmitWithoutToken(t *testing.T) {
	cfg := config.Config{
		Persister:       config.InMemoryPersister{},
		UserViperConfig: viper.New(),
	}

	err := runSubmit(cfg, pflag.NewFlagSet("fake", pflag.PanicOnError), []string{})
	assert.Regexp(t, "Welcome to Exercism", err.Error())
	assert.Regexp(t, "exercism.io/my/settings", err.Error())
}

func TestSubmitWithoutWorkspace(t *testing.T) {
	v := viper.New()
	v.Set("token", "abc123")

	cfg := config.Config{
		Persister:       config.InMemoryPersister{},
		UserViperConfig: v,
		DefaultBaseURL:  "http://example.com",
	}

	err := runSubmit(cfg, pflag.NewFlagSet("fake", pflag.PanicOnError), []string{})
	assert.Regexp(t, "re-run the configure", err.Error())
}

func TestSubmitNonExistentFile(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "submit-no-such-file")
	defer os.RemoveAll(tmpDir)
	assert.NoError(t, err)

	v := viper.New()
	v.Set("token", "abc123")
	v.Set("workspace", tmpDir)

	cfg := config.Config{
		Persister:       config.InMemoryPersister{},
		UserViperConfig: v,
		DefaultBaseURL:  "http://example.com",
	}

	err = ioutil.WriteFile(filepath.Join(tmpDir, "file-1.txt"), []byte("This is file 1"), os.FileMode(0755))
	assert.NoError(t, err)

	err = ioutil.WriteFile(filepath.Join(tmpDir, "file-2.txt"), []byte("This is file 2"), os.FileMode(0755))
	assert.NoError(t, err)
	files := []string{
		filepath.Join(tmpDir, "file-1.txt"),
		"no-such-file.txt",
		filepath.Join(tmpDir, "file-2.txt"),
	}
	err = runSubmit(cfg, pflag.NewFlagSet("fake", pflag.PanicOnError), files)
	assert.Regexp(t, "cannot be found", err.Error())
}

func TestSubmitExerciseWithoutSolutionMetadataFile(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "no-metadata-file")
	defer os.RemoveAll(tmpDir)
	assert.NoError(t, err)

	dir := filepath.Join(tmpDir, "bogus-track", "bogus-exercise")
	os.MkdirAll(dir, os.FileMode(0755))

	file := filepath.Join(dir, "file.txt")
	err = ioutil.WriteFile(file, []byte("This is a file."), os.FileMode(0755))
	assert.NoError(t, err)

	v := viper.New()
	v.Set("token", "abc123")
	v.Set("workspace", tmpDir)

	cfg := config.Config{
		Persister:       config.InMemoryPersister{},
		Dir:             tmpDir,
		UserViperConfig: v,
	}

	err = runSubmit(cfg, pflag.NewFlagSet("fake", pflag.PanicOnError), []string{file})
	assert.Error(t, err)
	assert.Regexp(t, "doesn't have the necessary metadata", err.Error())
}

func TestSubmitFilesAndDir(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "submit-no-such-file")
	defer os.RemoveAll(tmpDir)
	assert.NoError(t, err)

	v := viper.New()
	v.Set("token", "abc123")
	v.Set("workspace", tmpDir)

	cfg := config.Config{
		Persister:       config.InMemoryPersister{},
		UserViperConfig: v,
		DefaultBaseURL:  "http://example.com",
	}

	err = ioutil.WriteFile(filepath.Join(tmpDir, "file-1.txt"), []byte("This is file 1"), os.FileMode(0755))
	assert.NoError(t, err)

	err = ioutil.WriteFile(filepath.Join(tmpDir, "file-2.txt"), []byte("This is file 2"), os.FileMode(0755))
	assert.NoError(t, err)
	files := []string{
		filepath.Join(tmpDir, "file-1.txt"),
		tmpDir,
		filepath.Join(tmpDir, "file-2.txt"),
	}
	err = runSubmit(cfg, pflag.NewFlagSet("fake", pflag.PanicOnError), files)
	assert.Regexp(t, "submitting a directory", err.Error())
}

func TestSubmitFiles(t *testing.T) {
	oldOut := Out
	oldErr := Err
	Out = ioutil.Discard
	Err = ioutil.Discard
	defer func() {
		Out = oldOut
		Err = oldErr
	}()
	// The fake endpoint will populate this when it receives the call from the command.
	submittedFiles := map[string]string{}
	ts := fakeSubmitServer(t, submittedFiles)
	defer ts.Close()

	tmpDir, err := ioutil.TempDir("", "submit-files")
	defer os.RemoveAll(tmpDir)
	assert.NoError(t, err)

	dir := filepath.Join(tmpDir, "bogus-track", "bogus-exercise")
	os.MkdirAll(filepath.Join(dir, "subdir"), os.FileMode(0755))
	writeFakeSolution(t, dir, "bogus-track", "bogus-exercise")

	file1 := filepath.Join(dir, "file-1.txt")
	err = ioutil.WriteFile(file1, []byte("This is file 1."), os.FileMode(0755))
	assert.NoError(t, err)

	file2 := filepath.Join(dir, "subdir", "file-2.txt")
	err = ioutil.WriteFile(file2, []byte("This is file 2."), os.FileMode(0755))
	assert.NoError(t, err)

	// We don't filter *.md files if you explicitly pass the file path.
	readme := filepath.Join(dir, "README.md")
	err = ioutil.WriteFile(readme, []byte("This is the readme."), os.FileMode(0755))
	assert.NoError(t, err)

	v := viper.New()
	v.Set("token", "abc123")
	v.Set("workspace", tmpDir)
	v.Set("apibaseurl", ts.URL)

	cfg := config.Config{
		Persister:       config.InMemoryPersister{},
		Dir:             tmpDir,
		UserViperConfig: v,
	}

	files := []string{
		file1, file2, readme,
	}
	err = runSubmit(cfg, pflag.NewFlagSet("fake", pflag.PanicOnError), files)
	assert.NoError(t, err)

	assert.Equal(t, 3, len(submittedFiles))

	assert.Equal(t, "This is file 1.", submittedFiles["file-1.txt"])
	assert.Equal(t, "This is file 2.", submittedFiles["subdir/file-2.txt"])
	assert.Equal(t, "This is the readme.", submittedFiles["README.md"])
}

func TestLegacySolutionMetadataMigration(t *testing.T) {
	oldOut := Out
	oldErr := Err
	Out = ioutil.Discard
	Err = ioutil.Discard
	defer func() {
		Out = oldOut
		Err = oldErr
	}()
	// The fake endpoint will populate this when it receives the call from the command.
	submittedFiles := map[string]string{}
	ts := fakeSubmitServer(t, submittedFiles)
	defer ts.Close()

	tmpDir, err := ioutil.TempDir("", "legacy-metadata-file")
	defer os.RemoveAll(tmpDir)
	assert.NoError(t, err)

	dir := filepath.Join(tmpDir, "bogus-track", "bogus-exercise")
	os.MkdirAll(dir, os.FileMode(0755))

	// Write fake legacy solution
	solution := &workspace.Solution{
		ID:          "bogus-solution-uuid",
		Track:       "bogus-track",
		Exercise:    "bogus-exercise",
		URL:         "http://example.com/bogus-url",
		IsRequester: true,
	}
	b, err := json.Marshal(solution)
	assert.NoError(t, err)
	exercise := workspace.NewExerciseFromDir(dir)
	err = ioutil.WriteFile(exercise.LegacyMetadataFilepath(), b, os.FileMode(0600))
	assert.NoError(t, err)

	file := filepath.Join(dir, "file.txt")
	err = ioutil.WriteFile(file, []byte("This is a file."), os.FileMode(0755))
	assert.NoError(t, err)

	v := viper.New()
	v.Set("token", "abc123")
	v.Set("workspace", tmpDir)
	v.Set("apibaseurl", ts.URL)
	cfg := config.Config{
		Persister:       config.InMemoryPersister{},
		Dir:             tmpDir,
		UserViperConfig: v,
	}
	expectedPathAfterMigration := exercise.MetadataFilepath()
	_, err = os.Stat(expectedPathAfterMigration)
	assert.Error(t, err)

	err = runSubmit(cfg, pflag.NewFlagSet("fake", pflag.PanicOnError), []string{file})
	assert.NoError(t, err)
	assert.Equal(t, "This is a file.", submittedFiles["file.txt"])

	_, err = os.Stat(expectedPathAfterMigration)
	assert.NoError(t, err)
	_, err = os.Stat(exercise.LegacyMetadataFilepath())
	assert.Error(t, err)
}

func TestSubmitWithEmptyFile(t *testing.T) {
	oldOut := Out
	oldErr := Err
	Out = ioutil.Discard
	Err = ioutil.Discard
	defer func() {
		Out = oldOut
		Err = oldErr
	}()

	// The fake endpoint will populate this when it receives the call from the command.
	submittedFiles := map[string]string{}
	ts := fakeSubmitServer(t, submittedFiles)
	defer ts.Close()

	tmpDir, err := ioutil.TempDir("", "empty-file")
	defer os.RemoveAll(tmpDir)
	assert.NoError(t, err)

	dir := filepath.Join(tmpDir, "bogus-track", "bogus-exercise")
	os.MkdirAll(dir, os.FileMode(0755))

	writeFakeSolution(t, dir, "bogus-track", "bogus-exercise")

	v := viper.New()
	v.Set("token", "abc123")
	v.Set("workspace", tmpDir)
	v.Set("apibaseurl", ts.URL)

	cfg := config.Config{
		Persister:       config.InMemoryPersister{},
		UserViperConfig: v,
	}

	file1 := filepath.Join(dir, "file-1.txt")
	err = ioutil.WriteFile(file1, []byte(""), os.FileMode(0755))
	file2 := filepath.Join(dir, "file-2.txt")
	err = ioutil.WriteFile(file2, []byte("This is file 2."), os.FileMode(0755))

	err = runSubmit(cfg, pflag.NewFlagSet("fake", pflag.PanicOnError), []string{file1, file2})
	assert.NoError(t, err)

	assert.Equal(t, 1, len(submittedFiles))
	assert.Equal(t, "This is file 2.", submittedFiles["file-2.txt"])
}

func TestSubmitFilesForTeamExercise(t *testing.T) {
	oldOut := Out
	oldErr := Err
	Out = ioutil.Discard
	Err = ioutil.Discard
	defer func() {
		Out = oldOut
		Err = oldErr
	}()
	// The fake endpoint will populate this when it receives the call from the command.
	submittedFiles := map[string]string{}
	ts := fakeSubmitServer(t, submittedFiles)
	defer ts.Close()

	tmpDir, err := ioutil.TempDir("", "submit-files")
	assert.NoError(t, err)

	dir := filepath.Join(tmpDir, "teams", "bogus-team", "bogus-track", "bogus-exercise")
	os.MkdirAll(filepath.Join(dir, "subdir"), os.FileMode(0755))
	writeFakeSolution(t, dir, "bogus-track", "bogus-exercise")

	file1 := filepath.Join(dir, "file-1.txt")
	err = ioutil.WriteFile(file1, []byte("This is file 1."), os.FileMode(0755))
	assert.NoError(t, err)

	file2 := filepath.Join(dir, "subdir", "file-2.txt")
	err = ioutil.WriteFile(file2, []byte("This is file 2."), os.FileMode(0755))
	assert.NoError(t, err)

	v := viper.New()
	v.Set("token", "abc123")
	v.Set("workspace", tmpDir)
	v.Set("apibaseurl", ts.URL)

	cfg := config.Config{
		Dir:             tmpDir,
		UserViperConfig: v,
	}

	files := []string{
		file1, file2,
	}
	err = runSubmit(cfg, pflag.NewFlagSet("fake", pflag.PanicOnError), files)
	assert.NoError(t, err)

	assert.Equal(t, 2, len(submittedFiles))

	assert.Equal(t, "This is file 1.", submittedFiles["file-1.txt"])
	assert.Equal(t, "This is file 2.", submittedFiles["subdir/file-2.txt"])
}

func TestSubmitOnlyEmptyFile(t *testing.T) {
	oldOut := Out
	oldErr := Err
	Out = ioutil.Discard
	Err = ioutil.Discard
	defer func() {
		Out = oldOut
		Err = oldErr
	}()

	tmpDir, err := ioutil.TempDir("", "just-an-empty-file")
	defer os.RemoveAll(tmpDir)
	assert.NoError(t, err)

	dir := filepath.Join(tmpDir, "bogus-track", "bogus-exercise")
	os.MkdirAll(dir, os.FileMode(0755))

	writeFakeSolution(t, dir, "bogus-track", "bogus-exercise")

	v := viper.New()
	v.Set("token", "abc123")
	v.Set("workspace", tmpDir)

	cfg := config.Config{
		Persister:       config.InMemoryPersister{},
		UserViperConfig: v,
	}

	file := filepath.Join(dir, "file.txt")
	err = ioutil.WriteFile(file, []byte(""), os.FileMode(0755))

	err = runSubmit(cfg, pflag.NewFlagSet("fake", pflag.PanicOnError), []string{file})
	assert.Error(t, err)
	assert.Regexp(t, "No files found", err.Error())
}

func TestSubmitFilesFromDifferentSolutions(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "dir-1-submit")
	defer os.RemoveAll(tmpDir)
	assert.NoError(t, err)

	dir1 := filepath.Join(tmpDir, "bogus-track", "bogus-exercise-1")
	os.MkdirAll(dir1, os.FileMode(0755))
	writeFakeSolution(t, dir1, "bogus-track", "bogus-exercise-1")

	dir2 := filepath.Join(tmpDir, "bogus-track", "bogus-exercise-2")
	os.MkdirAll(dir2, os.FileMode(0755))
	writeFakeSolution(t, dir2, "bogus-track", "bogus-exercise-2")

	file1 := filepath.Join(dir1, "file-1.txt")
	err = ioutil.WriteFile(file1, []byte("This is file 1."), os.FileMode(0755))
	assert.NoError(t, err)

	file2 := filepath.Join(dir2, "file-2.txt")
	err = ioutil.WriteFile(file2, []byte("This is file 2."), os.FileMode(0755))
	assert.NoError(t, err)

	v := viper.New()
	v.Set("token", "abc123")
	v.Set("workspace", tmpDir)

	cfg := config.Config{
		Persister:       config.InMemoryPersister{},
		Dir:             tmpDir,
		UserViperConfig: v,
	}

	err = runSubmit(cfg, pflag.NewFlagSet("fake", pflag.PanicOnError), []string{file1, file2})
	assert.Error(t, err)
	assert.Regexp(t, "different solutions", err.Error())
}

func fakeSubmitServer(t *testing.T, submittedFiles map[string]string) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(2 << 10)
		if err != nil {
			t.Fatal(err)
		}
		mf := r.MultipartForm

		files := mf.File["files[]"]
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			body, err := ioutil.ReadAll(file)
			if err != nil {
				t.Fatal(err)
			}
			submittedFiles[fileHeader.Filename] = string(body)
		}
	})
	return httptest.NewServer(handler)
}

func TestSubmitRelativePath(t *testing.T) {
	oldOut := Out
	oldErr := Err
	Out = ioutil.Discard
	Err = ioutil.Discard
	defer func() {
		Out = oldOut
		Err = oldErr
	}()
	// The fake endpoint will populate this when it receives the call from the command.
	submittedFiles := map[string]string{}
	ts := fakeSubmitServer(t, submittedFiles)
	defer ts.Close()

	tmpDir, err := ioutil.TempDir("", "relative-path")
	defer os.RemoveAll(tmpDir)
	assert.NoError(t, err)

	dir := filepath.Join(tmpDir, "bogus-track", "bogus-exercise")
	os.MkdirAll(dir, os.FileMode(0755))

	writeFakeSolution(t, dir, "bogus-track", "bogus-exercise")

	v := viper.New()
	v.Set("token", "abc123")
	v.Set("workspace", tmpDir)
	v.Set("apibaseurl", ts.URL)

	cfg := config.Config{
		Persister:       config.InMemoryPersister{},
		UserViperConfig: v,
	}

	err = ioutil.WriteFile(filepath.Join(dir, "file.txt"), []byte("This is a file."), os.FileMode(0755))

	err = os.Chdir(dir)
	assert.NoError(t, err)

	err = runSubmit(cfg, pflag.NewFlagSet("fake", pflag.PanicOnError), []string{"file.txt"})
	assert.NoError(t, err)

	assert.Equal(t, 1, len(submittedFiles))
	assert.Equal(t, "This is a file.", submittedFiles["file.txt"])
}

func writeFakeSolution(t *testing.T, dir, trackID, exerciseSlug string) {
	solution := &workspace.Solution{
		ID:          "bogus-solution-uuid",
		Track:       trackID,
		Exercise:    exerciseSlug,
		URL:         "http://example.com/bogus-url",
		IsRequester: true,
	}
	err := solution.Write(dir)
	assert.NoError(t, err)
}
