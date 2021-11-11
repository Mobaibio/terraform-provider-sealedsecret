package git

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/assert"
)

const (
	testGitUrlKey      = "TEST_GIT_URL"
	testGitTokenKey    = "TEST_GIT_TOKEN"
	testGitUsernameKey = "TEST_GIT_USERNAME"
	testBranchName     = "test-branch-name"
)

func getEnv(t *testing.T, key string) string {
	value := os.Getenv(key)
	if value == "" {
		t.Fatalf("cannot run test when env var %s is empty", key)
	}
	return value
}

func TestCreateBranch(t *testing.T) {
	g := newGit(t, testBranchName)

	branches, err := g.repo.Branches()
	assert.Nil(t, err)

	var branchNames []string
	err = branches.ForEach(func(ref *plumbing.Reference) error {
		branchNames = append(branchNames, ref.Name().String())
		return nil
	})
	assert.Nil(t, err)

	isInSlice := func(branchName string) bool {
		for _, bn := range branchNames {
			if strings.Contains(bn, branchName) {
				return true
			}
		}
		return false
	}

	assert.True(t, isInSlice(testBranchName), fmt.Sprintf("expected to find a branch by name %s, got %v", testBranchName, branchNames))
}

func TestGit_Push(t *testing.T) {
	g := newGit(t, testBranchName)
	defer cleanupBranch(t, g)
	testPath, testFile := push(t, g)
	validatePush(t, g, testPath, testFile)
}

func TestGit_Push_BranchAlreadyExist(t *testing.T) {
	g := newGit(t, "main")
	testPath, testFile := push(t, g)
	validatePush(t, g, testPath, testFile)
}

func TestGit_ConcurrentPush(t *testing.T) {
	g := newGit(t, testBranchName)
	defer cleanupBranch(t, g)
	wg := &sync.WaitGroup{}
	const numberOfRequests = 5

	wg.Add(numberOfRequests)
	fn := func() {
		testPath, testFile := push(t, g)
		validatePush(t, g, testPath, testFile)
		wg.Done()
	}
	for i := 0; i < numberOfRequests; i++ {
		go fn()
	}
	wg.Wait()
}

func TestGit_DeleteFile(t *testing.T) {
	g := newGit(t, testBranchName)
	defer cleanupBranch(t, g)
	testPath, testFile := push(t, g)
	validatePush(t, g, testPath, testFile)

	err := g.DeleteFile(context.Background(), testPath)
	assert.Nil(t, err)

	fs := cloneBranch(t, g)
	_, err = fs.Open(testPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestGit_DeleteFile_NoExist(t *testing.T) {
	g := newGit(t, testBranchName)
	err := g.DeleteFile(context.Background(), "testpath/test.txt")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func push(t *testing.T, g *Git) (testPath string, testFile []byte) {
	testFile, testPath = []byte("my awesome test file"), "testpath/test.txt"
	err := g.Push(context.Background(), testFile, testPath)
	assert.Nil(t, err)

	return testPath, testFile
}

func validatePush(t *testing.T, g *Git, testPath string, testFile []byte) {
	fs := cloneBranch(t, g)
	file, err := fs.Open(testPath)
	assert.Nil(t, err)
	actualFile, err := io.ReadAll(file)
	assert.Nil(t, err)
	assert.True(t, bytes.Equal(testFile, actualFile))
}

func cloneBranch(t *testing.T, g *Git) billy.Filesystem {
	// we have to wait for gitlab to update
	time.Sleep(1 * time.Second)
	// updating the git instance to ensure that the file actually was pushed
	fs := memfs.New()
	_, err := git.Clone(memory.NewStorage(), fs, &git.CloneOptions{
		URL:           g.url,
		Auth:          g.auth,
		ReferenceName: plumbing.NewBranchReferenceName(g.sourceBranch),
		SingleBranch:  true,
	})
	assert.Nil(t, err)
	return fs
}

func cleanupBranch(t *testing.T, g *Git) {
	err := g.repo.Push(&git.PushOptions{
		Auth:     g.auth,
		RefSpecs: []config.RefSpec{config.RefSpec(":refs/heads/" + g.sourceBranch)},
		Progress: os.Stdout,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func newGit(t *testing.T, branchName string) *Git {
	g, err := NewGit(context.Background(), getEnv(t, testGitUrlKey), branchName, "main", BasicAuth{
		Username: getEnv(t, testGitUsernameKey),
		Token:    getEnv(t, testGitTokenKey),
	})
	assert.Nil(t, err)

	return g
}
