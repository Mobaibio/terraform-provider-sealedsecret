package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/akselleirv/sealedsecret/internal/gitlab"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

type Git struct {
	url          string
	sourceBranch string
	targetBranch string
	repo         *git.Repository
	fs           billy.Filesystem
	auth         *http.BasicAuth
	mu           *sync.Mutex
}

type BasicAuth struct {
	Username, Token string
}

const (
	remoteName = "origin"
)

type Giter interface {
	Push(ctx context.Context, file []byte, path string) error
	GetFile(filePath string) ([]byte, error)
	DeleteFile(ctx context.Context, filePath string) error
	CreateMergeRequest() error
}

func NewGit(ctx context.Context, url, sourceBranch, targetBranch string, auth BasicAuth) (*Git, error) {
	basicAuth := &http.BasicAuth{
		Username: auth.Username,
		Password: auth.Token,
	}
	fs := memfs.New()

	logDebug("Cloning Git repository with url " + url)
	r, err := git.CloneContext(ctx, memory.NewStorage(), fs, &git.CloneOptions{
		URL:  url,
		Auth: basicAuth,
	})
	if err != nil {
		return nil, err
	}

	if err = createBranch(r, sourceBranch); err != nil {
		return nil, err
	}

	return &Git{
		repo:         r,
		fs:           fs,
		auth:         basicAuth,
		url:          url,
		sourceBranch: sourceBranch,
		targetBranch: targetBranch,
		mu:           &sync.Mutex{},
	}, nil
}

// Push creates the new file and pushes the changes to Git remote.
//
// filePath must specify the path to where the new file should be created
func (g *Git) Push(ctx context.Context, file []byte, filePath string) error {
	// when multiple resources are created we need to update the git refs head after push
	g.mu.Lock()
	defer g.mu.Unlock()

	newFile, err := g.fs.Create(filePath)
	if err != nil {
		return fmt.Errorf("unable to create file: %w", err)
	}

	_, err = newFile.Write(file)
	if err != nil {
		return fmt.Errorf("unable to write to file: %w", err)
	}
	err = newFile.Close()
	if err != nil {
		return err
	}

	w, err := g.repo.Worktree()
	if err != nil {
		return err
	}

	if err != nil {
		return err
	}
	_, err = w.Add(filePath)
	if err != nil {
		return fmt.Errorf("unable to add: %w", err)
	}
	_, err = w.Commit(createCommitMsg("created", filePath), commitOpts())
	if err != nil {
		return fmt.Errorf("unable to commit: %w", err)
	}

	if err := g.repo.FetchContext(ctx, &git.FetchOptions{RemoteName: remoteName, Auth: g.auth}); err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("unable to fetch: %w", err)
	}

	if err := g.repo.PushContext(ctx, &git.PushOptions{RemoteName: remoteName, Auth: g.auth, Force: true}); err != nil {
		return fmt.Errorf("unable to push: %w", err)
	}

	return nil
}

func (g *Git) GetFile(filePath string) ([]byte, error) {
	f, err := g.fs.Open(filePath)
	if err != nil {
		return nil, err
	}

	return io.ReadAll(f)
}

func (g *Git) DeleteFile(ctx context.Context, filePath string) error {
	// when multiple resources are created we need to update the git refs head after push
	g.mu.Lock()
	defer g.mu.Unlock()

	w, err := g.repo.Worktree()
	if err != nil {
		return err
	}
	_, err = w.Remove(filePath)
	if err != nil && errors.Is(err, index.ErrEntryNotFound) {
		return os.ErrNotExist
	}
	if err != nil {
		return err
	}
	_, err = w.Commit(createCommitMsg("deleted", filePath), commitOpts())
	if err != nil {
		return err
	}
	if err := g.repo.PushContext(ctx, &git.PushOptions{RemoteName: remoteName, Auth: g.auth}); err != nil {
		return err
	}

	if err := g.repo.FetchContext(ctx, &git.FetchOptions{RemoteName: remoteName, Auth: g.auth}); err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return err
	}
	return nil
}

func (g *Git) CreateMergeRequest() error {
	return gitlab.CreateMergeRequest(g.url, g.auth.Password, g.sourceBranch, g.targetBranch)
}

func createCommitMsg(action, filePath string) string {
	return fmt.Sprintf("[SEALEDSECRET-PROVIDER] %s --> %s", action, filePath)
}

func commitOpts() *git.CommitOptions {
	return &git.CommitOptions{
		Author: &object.Signature{
			Name: "SEALEDSECRET-PROVIDER",
			When: time.Now(),
		}}
}

// createBranch creates a branch if it does not exist and ignores the call if it exists.
func createBranch(r *git.Repository, branchName string) error {
	wt, err := r.Worktree()
	if err != nil {
		return err
	}
	err = wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
		Create: true,
	})
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			logDebug("Reusing branch " + branchName)
			return wt.Checkout(&git.CheckoutOptions{
				Branch: plumbing.NewBranchReferenceName(branchName),
				Create: false,
			})
		}
		return err
	}
	logDebug("Creating branch with name " + branchName)
	return err
}

func logDebug(msg string) {
	log.Printf("[DEBUG] %s\n", msg)
}
