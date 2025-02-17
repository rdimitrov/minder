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

// Package trusty provides an evaluator that uses the trusty API
package trusty

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/stacklok/minder/internal/engine/eval/pr_actions"
	engif "github.com/stacklok/minder/internal/engine/interfaces"
	"github.com/stacklok/minder/internal/providers"
	pb "github.com/stacklok/minder/pkg/api/protobuf/go/minder/v1"
	provifv1 "github.com/stacklok/minder/pkg/providers/v1"
)

const (
	// TrustyEvalType is the type of the trusty evaluator
	TrustyEvalType = "trusty"
)

// Evaluator is the trusty evaluator
type Evaluator struct {
	cli      provifv1.GitHub
	endpoint string
}

// NewTrustyEvaluator creates a new trusty evaluator
func NewTrustyEvaluator(
	pie *pb.RuleType_Definition_Eval_Trusty,
	pbuild *providers.ProviderBuilder,
) (*Evaluator, error) {
	if pbuild == nil {
		return nil, fmt.Errorf("provider builder is nil")
	}

	if pie.GetEndpoint() == "" {
		return nil, fmt.Errorf("endpoint is not set")
	}

	ghcli, err := pbuild.GetGitHub(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get github client: %w", err)
	}

	return &Evaluator{
		cli:      ghcli,
		endpoint: pie.GetEndpoint(),
	}, nil
}

// Eval implements the Evaluator interface.
func (e *Evaluator) Eval(ctx context.Context, pol map[string]any, res *engif.Result) error {
	var evalErr error

	//nolint:govet
	prdeps, ok := res.Object.(pb.PrDependencies)
	if !ok {
		return fmt.Errorf("invalid object type for vulncheck evaluator")
	}

	if len(prdeps.Deps) == 0 {
		return nil
	}

	ruleConfig, err := parseConfig(pol)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if !isActionImplemented(ruleConfig.Action) {
		return fmt.Errorf("action %s is not implemented", ruleConfig.Action)
	}

	logger := zerolog.Ctx(ctx).With().
		Int32("pull-number", prdeps.Pr.Number).
		Str("repo-owner", prdeps.Pr.RepoOwner).
		Str("repo-name", prdeps.Pr.RepoName).
		Logger()

	prSummaryHandler, err := newSummaryPrHandler(prdeps.Pr, e.cli, e.endpoint)
	if err != nil {
		return fmt.Errorf("failed to create summary handler: %w", err)
	}

	piCli := newPiClient(e.endpoint)
	if piCli == nil {
		return fmt.Errorf("failed to create pi client")
	}

	for _, dep := range prdeps.Deps {
		ecoConfig := ruleConfig.getEcosystemConfig(dep.Dep.Ecosystem)
		if ecoConfig == nil {
			logger.Info().
				Str("dependency", dep.Dep.Name).
				Str("ecosystem", dep.Dep.Ecosystem.AsString()).
				Msgf("no config for ecosystem, skipping")
			continue
		}

		resp, err := piCli.SendRecvRequest(ctx, dep.Dep)
		if err != nil {
			return fmt.Errorf("failed to send request: %w", err)
		}

		if resp == nil || resp.PackageName == "" {
			logger.Info().
				Str("dependency", dep.Dep.Name).
				Msgf("no trusty data for dependency, skipping")
			continue
		}

		if resp.Summary.Score == 0 {
			logger.Info().
				Str("dependency", dep.Dep.Name).
				Msgf("the dependency has no score, skipping")
			continue
		}

		if resp.Summary.Score >= ecoConfig.Score {
			logger.Debug().
				Str("dependency", dep.Dep.Name).
				Float64("pkgScore", resp.Summary.Score).
				Float64("threshold", ecoConfig.Score).
				Msgf("the dependency has higher score than threshold, skipping")
			continue
		}

		logger.Debug().
			Str("dependency", dep.Dep.Name).
			Float64("pkgScore", resp.Summary.Score).
			Float64("threshold", ecoConfig.Score).
			Msgf("the dependency has lower score than threshold, tracking")

		evalErr = fmt.Errorf("score for %s is %f is lower than threshold %f",
			dep.Dep.Name, resp.Summary.Score, ecoConfig.Score)

		prSummaryHandler.trackAlternatives(dep, resp)
	}

	if err := prSummaryHandler.submit(ctx); err != nil {
		return fmt.Errorf("failed to submit summary: %w", err)
	}

	return evalErr
}

func isActionImplemented(action pr_actions.Action) bool {
	return action == pr_actions.ActionSummary
}
