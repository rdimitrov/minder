//
// Copyright 2023 Stacklok, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// NOTE: This file is for stubbing out client code for proof of concept
// purposes. It will / should be removed in the future.
// Until then, it is not covered by unit tests and should not be used
// It does make a good example of how to use the generated client code
// for others to use as a reference.

package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/stacklok/minder/internal/util/rand"
)

type RepositoryOption func(*CreateRepositoryParams)

func deleteRepositoryByRepoId(params CreateRepositoryParams) error {
	repo, err := testQueries.GetRepositoryByRepoID(
		context.Background(), params.RepoID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return testQueries.DeleteRepository(context.Background(), repo.ID)
}

func createRandomRepository(t *testing.T, project uuid.UUID, prov string, opts ...RepositoryOption) Repository {
	t.Helper()

	seed := time.Now().UnixNano()
	arg := CreateRepositoryParams{
		Provider:   prov,
		ProjectID:  project,
		RepoOwner:  rand.RandomName(seed),
		RepoName:   rand.RandomName(seed),
		RepoID:     int32(rand.RandomInt(0, 1000, seed)),
		IsPrivate:  false,
		IsFork:     false,
		WebhookID:  sql.NullInt32{Int32: int32(rand.RandomInt(0, 1000, seed)), Valid: true},
		WebhookUrl: rand.RandomURL(seed),
		DeployUrl:  rand.RandomURL(seed),
	}
	// Allow arbitrary fixups to the Repository
	for _, o := range opts {
		o(&arg)
	}

	// Avoid unique constraint violation
	require.NoError(t, deleteRepositoryByRepoId(arg))

	repo, err := testQueries.CreateRepository(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, repo)

	require.Equal(t, arg.Provider, repo.Provider)
	require.Equal(t, arg.ProjectID, repo.ProjectID)
	require.Equal(t, arg.RepoOwner, repo.RepoOwner)
	require.Equal(t, arg.RepoName, repo.RepoName)
	require.Equal(t, arg.RepoID, repo.RepoID)
	require.Equal(t, arg.IsPrivate, repo.IsPrivate)
	require.Equal(t, arg.IsFork, repo.IsFork)
	require.Equal(t, arg.WebhookID, repo.WebhookID)
	require.Equal(t, arg.WebhookUrl, repo.WebhookUrl)

	require.NotZero(t, repo.ID)
	require.NotZero(t, repo.ProjectID)
	require.NotZero(t, repo.CreatedAt)
	require.NotZero(t, repo.UpdatedAt)

	return repo
}

func TestRepository(t *testing.T) {
	t.Parallel()

	org := createRandomOrganization(t)
	group := createRandomProject(t, org.ID)
	prov := createRandomProvider(t, group.ID)
	createRandomRepository(t, group.ID, prov.Name)
}

func TestGetRepositoryByID(t *testing.T) {
	t.Parallel()

	org := createRandomOrganization(t)
	group := createRandomProject(t, org.ID)
	prov := createRandomProvider(t, group.ID)
	repo1 := createRandomRepository(t, group.ID, prov.Name)

	repo2, err := testQueries.GetRepositoryByID(context.Background(), repo1.ID)
	require.NoError(t, err)
	require.NotEmpty(t, repo2)

	require.Equal(t, repo1.ID, repo2.ID)
	require.Equal(t, repo1.Provider, repo2.Provider)
	require.Equal(t, repo1.ProjectID, repo2.ProjectID)
	require.Equal(t, repo1.RepoOwner, repo2.RepoOwner)
	require.Equal(t, repo1.RepoName, repo2.RepoName)
	require.Equal(t, repo1.RepoID, repo2.RepoID)
	require.Equal(t, repo1.IsPrivate, repo2.IsPrivate)
	require.Equal(t, repo1.IsFork, repo2.IsFork)
	require.Equal(t, repo1.WebhookID, repo2.WebhookID)
	require.Equal(t, repo1.WebhookUrl, repo2.WebhookUrl)
	require.Equal(t, repo1.DeployUrl, repo2.DeployUrl)
	require.Equal(t, repo1.CreatedAt, repo2.CreatedAt)
	require.Equal(t, repo1.UpdatedAt, repo2.UpdatedAt)
}

func TestGetRepositoryByRepoName(t *testing.T) {
	t.Parallel()

	org := createRandomOrganization(t)
	group := createRandomProject(t, org.ID)
	prov := createRandomProvider(t, group.ID)
	repo1 := createRandomRepository(t, group.ID, prov.Name)

	repo2, err := testQueries.GetRepositoryByRepoName(context.Background(), GetRepositoryByRepoNameParams{
		Provider: repo1.Provider, RepoOwner: repo1.RepoOwner, RepoName: repo1.RepoName})
	require.NoError(t, err)
	require.NotEmpty(t, repo2)

	require.Equal(t, repo1.ID, repo2.ID)
	require.Equal(t, repo1.Provider, repo2.Provider)
	require.Equal(t, repo1.ProjectID, repo2.ProjectID)
	require.Equal(t, repo1.RepoOwner, repo2.RepoOwner)
	require.Equal(t, repo1.RepoName, repo2.RepoName)
	require.Equal(t, repo1.RepoID, repo2.RepoID)
	require.Equal(t, repo1.IsPrivate, repo2.IsPrivate)
	require.Equal(t, repo1.IsFork, repo2.IsFork)
	require.Equal(t, repo1.WebhookID, repo2.WebhookID)
	require.Equal(t, repo1.WebhookUrl, repo2.WebhookUrl)
	require.Equal(t, repo1.DeployUrl, repo2.DeployUrl)
	require.Equal(t, repo1.CreatedAt, repo2.CreatedAt)
	require.Equal(t, repo1.UpdatedAt, repo2.UpdatedAt)
}

func TestListRepositoriesByProjectID(t *testing.T) {
	t.Parallel()

	org := createRandomOrganization(t)
	group := createRandomProject(t, org.ID)
	prov := createRandomProvider(t, group.ID)
	createRandomRepository(t, group.ID, prov.Name)

	for i := int32(1001); i < 1020; i++ {
		createRandomRepository(t, group.ID, prov.Name, func(r *CreateRepositoryParams) {
			r.RepoID = int32(i)
		})
	}

	arg := ListRepositoriesByProjectIDParams{
		Provider:  prov.Name,
		ProjectID: group.ID,
	}

	repos, err := testQueries.ListRepositoriesByProjectID(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, repos)

	for _, repo := range repos {
		require.NotEmpty(t, repo)
		require.Equal(t, arg.ProjectID, repo.ProjectID)
	}
}

func TestUpdateRepository(t *testing.T) {
	t.Parallel()

	org := createRandomOrganization(t)
	group := createRandomProject(t, org.ID)
	prov := createRandomProvider(t, group.ID)
	repo1 := createRandomRepository(t, group.ID, prov.Name)

	arg := UpdateRepositoryParams{
		ID:         repo1.ID,
		Provider:   repo1.Provider,
		ProjectID:  repo1.ProjectID,
		RepoOwner:  repo1.RepoOwner,
		RepoName:   repo1.RepoName,
		RepoID:     repo1.RepoID,
		IsPrivate:  repo1.IsPrivate,
		IsFork:     repo1.IsFork,
		WebhookID:  sql.NullInt32{Int32: 1234, Valid: true},
		WebhookUrl: repo1.WebhookUrl,
		DeployUrl:  repo1.DeployUrl,
	}

	repo2, err := testQueries.UpdateRepository(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, repo2)

	require.Equal(t, repo1.ID, repo2.ID)
	require.Equal(t, repo1.Provider, repo2.Provider)
	require.Equal(t, repo1.ProjectID, repo2.ProjectID)
	require.Equal(t, repo1.RepoOwner, repo2.RepoOwner)
	require.Equal(t, repo1.RepoName, repo2.RepoName)
	require.Equal(t, repo1.RepoID, repo2.RepoID)
	require.Equal(t, repo1.IsPrivate, repo2.IsPrivate)
	require.Equal(t, repo1.IsFork, repo2.IsFork)
	require.Equal(t, arg.WebhookID, repo2.WebhookID)
	require.Equal(t, repo1.WebhookUrl, repo2.WebhookUrl)
	require.Equal(t, repo1.DeployUrl, repo2.DeployUrl)
	require.Equal(t, repo1.CreatedAt, repo2.CreatedAt)
	require.NotEqual(t, repo1.UpdatedAt, repo2.UpdatedAt)
}

func TestUpdateRepositoryByRepoId(t *testing.T) {
	t.Parallel()

	org := createRandomOrganization(t)
	group := createRandomProject(t, org.ID)
	prov := createRandomProvider(t, group.ID)
	repo1 := createRandomRepository(t, group.ID, prov.Name)

	arg := UpdateRepositoryByIDParams{
		RepoID:     repo1.RepoID,
		Provider:   repo1.Provider,
		ProjectID:  repo1.ProjectID,
		RepoOwner:  repo1.RepoOwner,
		RepoName:   repo1.RepoName,
		IsPrivate:  repo1.IsPrivate,
		IsFork:     repo1.IsFork,
		WebhookID:  sql.NullInt32{Int32: 1234, Valid: true},
		WebhookUrl: repo1.WebhookUrl,
		DeployUrl:  repo1.DeployUrl,
	}

	repo2, err := testQueries.UpdateRepositoryByID(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, repo2)

	require.Equal(t, repo1.ID, repo2.ID)
	require.Equal(t, repo1.Provider, repo2.Provider)
	require.Equal(t, repo1.ProjectID, repo2.ProjectID)
	require.Equal(t, repo1.RepoOwner, repo2.RepoOwner)
	require.Equal(t, repo1.RepoName, repo2.RepoName)
	require.Equal(t, repo1.RepoID, repo2.RepoID)
	require.Equal(t, repo1.IsPrivate, repo2.IsPrivate)
	require.Equal(t, repo1.IsFork, repo2.IsFork)
	require.Equal(t, arg.WebhookID, repo2.WebhookID)
	require.Equal(t, repo1.WebhookUrl, repo2.WebhookUrl)
	require.Equal(t, repo1.DeployUrl, repo2.DeployUrl)
	require.Equal(t, repo1.CreatedAt, repo2.CreatedAt)
	require.NotEqual(t, repo1.UpdatedAt, repo2.UpdatedAt)
}

func TestDeleteRepository(t *testing.T) {
	t.Parallel()

	org := createRandomOrganization(t)
	group := createRandomProject(t, org.ID)
	prov := createRandomProvider(t, group.ID)
	repo1 := createRandomRepository(t, group.ID, prov.Name)

	err := testQueries.DeleteRepository(context.Background(), repo1.ID)
	require.NoError(t, err)

	repo2, err := testQueries.GetRepositoryByID(context.Background(), repo1.ID)
	require.Error(t, err)
	require.EqualError(t, err, sql.ErrNoRows.Error())
	require.Empty(t, repo2)
}
