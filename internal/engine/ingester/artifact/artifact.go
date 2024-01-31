// Copyright 2023 Stacklok, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// Package rule provides the CLI subcommand for managing rules

// Package artifact provides the artifact ingestion engine
package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	evalerrors "github.com/stacklok/minder/internal/engine/errors"
	engif "github.com/stacklok/minder/internal/engine/interfaces"
	"github.com/stacklok/minder/internal/providers"
	"github.com/stacklok/minder/internal/verifier"
	"github.com/stacklok/minder/internal/verifier/sigstore/container"
	"github.com/stacklok/minder/internal/verifier/verifyif"
	pb "github.com/stacklok/minder/pkg/api/protobuf/go/minder/v1"
	provifv1 "github.com/stacklok/minder/pkg/providers/v1"
)

const (
	// ArtifactRuleDataIngestType is the type of the artifact rule data ingest engine
	ArtifactRuleDataIngestType = "artifact"
)

// Ingest is the engine for a rule type that uses artifact data ingest
// Implements enginer.ingester.Ingester
type Ingest struct {
	ghCli provifv1.GitHub
}

type verification struct {
	IsSigned          bool   `json:"is_signed"`
	IsVerified        bool   `json:"is_verified"`
	IsBundleVerified  bool   `json:"is_bundle_verified"`
	Repository        string `json:"repository"`
	Branch            string `json:"branch"`
	WorkflowName      string `json:"workflow_name"`
	RunnerEnvironment string `json:"runner_environment"`
	CertIssuer        string `json:"cert_issuer"`
}

type verificationResult struct {
	Verification verification `json:"Verification"`
}

// NewArtifactDataIngest creates a new artifact rule data ingest engine
func NewArtifactDataIngest(
	_ *pb.ArtifactType,
	pbuild *providers.ProviderBuilder,
) (*Ingest, error) {

	ghCli, err := pbuild.GetGitHub(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get github client: %w", err)
	}

	return &Ingest{
		ghCli: ghCli,
	}, nil
}

// GetType returns the type of the artifact rule data ingest engine
func (*Ingest) GetType() string {
	return ArtifactRuleDataIngestType
}

// GetConfig returns the config for the artifact rule data ingest engine
func (*Ingest) GetConfig() proto.Message {
	return nil
}

// Ingest checks the passed in artifact, makes sure it is applicable to the current rule
// and if it is, returns the appropriately marshalled data.
func (i *Ingest) Ingest(
	ctx context.Context,
	ent proto.Message,
	params map[string]any,
) (*engif.Result, error) {
	cfg, err := configFromParams(params)
	if err != nil {
		return nil, err
	}

	artifact, ok := ent.(*pb.Artifact)
	if !ok {
		return nil, fmt.Errorf("expected Artifact, got %T", ent)
	}

	// Filter the versions of the artifact that are applicable to this rule
	applicable, err := i.getApplicableArtifactVersions(ctx, artifact, cfg)
	if err != nil {
		return nil, err
	}

	return &engif.Result{
		Object: applicable,
	}, nil
}

// in the artifact we need:
// - owner login
// - name
// - type
// - version.tags
// - version.name
func (i *Ingest) getApplicableArtifactVersions(
	ctx context.Context,
	artifact *pb.Artifact,
	cfg *ingesterConfig,
) ([]map[string]any, error) {
	// make sure the artifact type matches
	if newArtifactIngestType(artifact.Type) != cfg.Type {
		return nil, evalerrors.NewErrEvaluationSkipSilently("artifact type mismatch")
	}

	// if a name is specified, make sure it matches
	if cfg.Name != "" && cfg.Name != artifact.Name {
		return nil, evalerrors.NewErrEvaluationSkipSilently("artifact name mismatch")
	}

	// Build a tag matcher based on the configuration
	tagMatcher, err := buildTagMatcher(cfg.Tags, cfg.TagRegex)
	if err != nil {
		return nil, err
	}

	// get all versions of the artifact that are applicable to this rule
	versions, err := getArtifactVersions(ctx, i.ghCli, artifact)
	if err != nil {
		return nil, err
	}

	versionResults := make([]verificationResult, 0, len(versions))
	for _, artifactVersion := range versions {
		if tagMatcher.MatchTag(artifactVersion.Tags...) {
			res, err := getVerificationResult(
				ctx, i.ghCli, artifact.Owner, artifact.Name, artifactVersion.Sha, cfg.Sigstore)
			if err != nil {
				return nil, err
			}
			versionResults = append(versionResults, verificationResult{
				Verification: verification{
					IsSigned:          res.IsSigned,
					IsVerified:        res.IsVerified,
					IsBundleVerified:  res.IsBundleVerified,
					Repository:        res.Signature.Certificate.SourceRepositoryURI,
					Branch:            branchFromRef(res.Signature.Certificate.SourceRepositoryRef),
					WorkflowName:      res.Signature.Certificate.BuildSignerURI,
					RunnerEnvironment: res.Signature.Certificate.RunnerEnvironment,
					CertIssuer:        res.Signature.Certificate.Issuer,
				},
			})
		}
	}

	// if no applicable artifact versions were found for this rule, we can go ahead and fail the rule evaluation here
	if len(versionResults) == 0 {
		return nil, evalerrors.NewErrEvaluationFailed("no applicable artifact versions found")
	}

	jsonBytes, err := json.Marshal(versionResults)
	if err != nil {
		return nil, err
	}

	result := make([]map[string]any, 0, len(versionResults))
	err = json.Unmarshal(jsonBytes, &result)
	if err != nil {
		return nil, err
	}

	// return the list of applicable artifact versions
	return result, nil
}

func getArtifactVersions(ctx context.Context, ghCli provifv1.GitHub, artifact *pb.Artifact) ([]*pb.ArtifactVersion, error) {
	// if the artifact has versions, use them - this is processing a webhook request where it will
	// be just one version
	if artifact.Versions != nil {
		return artifact.Versions, nil
	}

	// if we don't have the versions, get them all from the API
	// now query for versions, retrieve the ones from last month
	isOrg := (ghCli.GetOwner() != "")
	upstreamVersions, err := ghCli.GetPackageVersions(ctx, isOrg, artifact.Owner, artifact.GetType(), artifact.GetName())
	if err != nil {
		return nil, fmt.Errorf("error retrieving artifact versions: %w", err)
	}

	pbVersions := make([]*pb.ArtifactVersion, 0, len(upstreamVersions))
	for _, version := range upstreamVersions {
		tags := version.Metadata.Container.Tags
		sort.Strings(tags)

		err = IsSkippable(verifier.ArtifactTypeContainer, version.CreatedAt.Time, map[string]interface{}{"tags": tags})
		if err != nil {
			zerolog.Ctx(ctx).Debug().Msg("skipping artifact version")
			continue
		}

		pbVersions = append(pbVersions, &pb.ArtifactVersion{
			VersionId: 0, // FIXME: this is a DB PK. Will be removed in a later commit
			Tags:      tags,
			Sha:       *version.Name,
			CreatedAt: timestamppb.New(version.CreatedAt.Time),
		})
	}

	return pbVersions, nil
}

func getVerificationResult(
	ctx context.Context,
	client provifv1.GitHub,
	artifactOwnerLogin, artifactName, sha, sigstoreURL string,
) (*verifyif.Result, error) {
	// get the verifier for sigstore
	artifactVerifier, err := verifier.NewVerifier(
		verifier.VerifierSigstore,
		sigstoreURL,
		container.WithAccessToken(client.GetToken()), container.WithGitHubClient(client))
	if err != nil {
		return nil, fmt.Errorf("error getting sigstore verifier: %w", err)
	}
	defer artifactVerifier.ClearCache()

	// now get information for signature and workflow
	res, err := artifactVerifier.Verify(ctx, verifier.ArtifactTypeContainer, "",
		artifactOwnerLogin, artifactName, sha)
	if err != nil {
		// FIXME: safer to return a special error here and catch it in the caller
		zerolog.Ctx(ctx).Debug().Err(err).Str("URI", res.GetUri()).Msg("no signature information found")
		return nil, err
	}

	return res, nil
}

var (
	// ArtifactTypeContainerRetentionPeriod represents the retention period for container artifacts
	ArtifactTypeContainerRetentionPeriod = time.Now().AddDate(0, -6, 0)
)

// IsSkippable determines if an artifact should be skipped
// TODO - this should be refactored as well, for now just a forklift from reconciler
func IsSkippable(artifactType verifier.ArtifactType, createdAt time.Time, opts map[string]interface{}) error {
	switch artifactType {
	case verifier.ArtifactTypeContainer:
		// if the artifact is older than the retention period, skip it
		if createdAt.Before(ArtifactTypeContainerRetentionPeriod) {
			return fmt.Errorf("artifact is older than retention period - %s", ArtifactTypeContainerRetentionPeriod)
		}
		tags, ok := opts["tags"].([]string)
		if !ok {
			return nil
		} else if len(tags) == 0 {
			// if the artifact has no tags, skip it
			return fmt.Errorf("artifact has no tags")
		}
		// if the artifact has a .sig tag it's a signature, skip it
		if verifier.GetSignatureTag(tags) != "" {
			return fmt.Errorf("artifact is a signature")
		}
		return nil
	default:
		return nil
	}
}

func branchFromRef(ref string) string {
	if strings.HasPrefix(ref, "refs/heads/") {
		return ref[len("refs/heads/"):]
	}

	return ""
}
