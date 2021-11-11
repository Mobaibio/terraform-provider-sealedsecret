package gitlab

import (
	"errors"
	"fmt"
	gl "github.com/xanzy/go-gitlab"
	"strings"
)

func CreateMergeRequest(url, token, sourceBranch, targetBranch string) error {
	git, err := gl.NewClient(token)
	if err != nil {
		return fmt.Errorf("unable to create new gitlab client: %w", err)
	}

	pid, err := getProjectId(url, git)
	if err != nil {
		return err
	}
	_, _, err = git.MergeRequests.CreateMergeRequest(pid, createMergeRequestOpts(targetBranch, sourceBranch))
	if err != nil {
		var errResp *gl.ErrorResponse
		errors.As(err, &errResp)
		// we want to make the command idempotent
		if strings.Contains(errResp.Message, "Another open merge request already exists for this source branch") {
			return nil
		}
		return fmt.Errorf("unable to create merge request: %w", err)
	}
	return nil
}

func getProjectId(url string, c *gl.Client) (int, error) {
	projects, _, err := c.Projects.ListProjects(createListProjectsOptions(url))
	if err != nil {
		return 0, fmt.Errorf("unable to get projects: %w", err)
	}
	for _, project := range projects {
		if project.WebURL == url {
			return project.ID, nil
		}
	}
	return 0, fmt.Errorf("unable to find any project for url %s", url)
}

func createMergeRequestOpts(targetBranch, sourceBranch string) *gl.CreateMergeRequestOptions {
	var (
		title              = "SealedSecrets update"
		description        = "This MR was automatically created by the terraform-provider-sealedsecrets."
		removeSourceBranch = true
	)
	var (
		titlePtr              *string
		targetBranchPtr       *string
		sourceBranchPtr       *string
		descriptionPtr        *string
		removeSourceBranchPtr *bool
	)

	targetBranchPtr = &targetBranch
	sourceBranchPtr = &sourceBranch
	titlePtr = &title
	descriptionPtr = &description
	removeSourceBranchPtr = &removeSourceBranch

	return &gl.CreateMergeRequestOptions{
		Title:              titlePtr,
		Description:        descriptionPtr,
		SourceBranch:       sourceBranchPtr,
		TargetBranch:       targetBranchPtr,
		RemoveSourceBranch: removeSourceBranchPtr,
	}

}

func createListProjectsOptions(url string) *gl.ListProjectsOptions {
	valueTrue := true
	s := strings.Split(url, "/")
	repoName := s[len(s)-1]
	var (
		truePtr   *bool
		searchPtr *string
	)

	truePtr = &valueTrue
	searchPtr = &repoName

	return &gl.ListProjectsOptions{Membership: truePtr, Search: searchPtr}
}
