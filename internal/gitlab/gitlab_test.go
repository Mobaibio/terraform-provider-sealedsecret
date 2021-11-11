package gitlab

import (
	"github.com/stretchr/testify/assert"
	"os"
	"sync"
	"testing"
)

const (
	testGitUrlKey   = "TEST_GIT_URL"
	testGitTokenKey = "TEST_GIT_TOKEN"
	testBranchName  = "test-branch-name"
)

// This test is not cleaning up after itself.
func TestCreateMergeRequest(t *testing.T) {
	token, url := getEnv(t, testGitTokenKey), getEnv(t, testGitUrlKey)
	assert.Nil(t, CreateMergeRequest(url, token, testBranchName+"-0", "main"))
}

// This test is not cleaning up after itself.
func TestConcurrentCreateMergeRequest(t *testing.T) {
	token, url := getEnv(t, testGitTokenKey), getEnv(t, testGitUrlKey)

	const numberOfRequests = 5
	wg := &sync.WaitGroup{}
	wg.Add(numberOfRequests)

	fn := func() {
		err := CreateMergeRequest(url, token, testBranchName+"-0", "main")
		assert.Nil(t, err)
		wg.Done()
	}

	for i := 0; i < numberOfRequests; i++ {
		go fn()
	}
	wg.Wait()
}

func getEnv(t *testing.T, key string) string {
	value := os.Getenv(key)
	if value == "" {
		t.Fatalf("cannot run test when env var %s is empty", key)
	}
	return value
}
