package usecase_test

import (
	"encoding/base64"
	"testing"

	"github.com/google/go-github/v29/github"
	"github.com/m-mizutani/octovy/backend/pkg/domain/interfaces"
	"github.com/m-mizutani/octovy/backend/pkg/domain/model"
	"github.com/m-mizutani/octovy/backend/pkg/infra/aws"
	"github.com/m-mizutani/octovy/backend/pkg/infra/githubapp"
	"github.com/m-mizutani/octovy/backend/pkg/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupFeedbackScanResult(t *testing.T) (interfaces.Usecases, *mockSet) {
	pem := genRSAKey(t)
	base64PEM := base64.StdEncoding.EncodeToString(pem)
	const secretsARN = "arn:aws:secretsmanager:us-east-0:123456789012:secret:tutorials/MyFirstSecret-jiObOV"

	// mocking DB
	dbClient := newTestTable(t)

	cfg := &model.Config{
		SecretsARN:     secretsARN,
		GitHubEndpoint: "https://ghe.example.org/api/v3",
		TableName:      dbClient.TableName(),
	}

	// Build service and injects mocks
	uc := usecase.New(cfg)
	svc := usecase.ExposeService(uc)
	svc.Infra.NewDB = func(region, tableName string) (interfaces.DBClient, error) {
		return dbClient, nil
	}

	// mocking SecretsManager
	newSM, mockSM := aws.NewMockSecretsManagerSet()
	mockSM.OutData[secretsARN] = map[string]string{
		"github_app_private_key": base64PEM,
		"github_app_id":          "123",
	}
	svc.Infra.NewSecretManager = newSM

	// Setup S3 mock
	newS3Mock, s3Mock := aws.NewMockS3()
	svc.Infra.NewS3 = newS3Mock
	s3Mock.Objects["my-db-bucket"] = map[string][]byte{
		"test-prefix/db/trivy.db.gz": []byte("boom!"),
	}

	// Setup GitHubApp mock
	newGitHubAppMock, gitHubAppMock := githubapp.NewMock()
	svc.Infra.NewGitHubApp = newGitHubAppMock

	// Setup trivy DB
	t.Cleanup(func() {
		assert.Equal(t, int64(123), gitHubAppMock.AppID)
		assert.Equal(t, "https://ghe.example.org/api/v3", gitHubAppMock.Endpoint)
	})

	return uc, &mockSet{
		db:        dbClient,
		githubapp: gitHubAppMock,
	}
}

func TestFeedbackScanResult(t *testing.T) {
	uc, mock := setupFeedbackScanResult(t)
	branch := model.GitHubBranch{
		GitHubRepo: model.GitHubRepo{
			Owner:    "clock",
			RepoName: "tower",
		},
		Branch: "main",
	}

	req := &model.FeedbackRequest{
		ReportID:  "abc",
		InstallID: 123,
		Options: model.FeedbackOptions{
			PullReqID:     model.Int(666),
			PullReqBranch: "main",
		},
	}

	oldReport := &model.ScanReport{
		ReportID:  "ebc",
		ScannedAt: 1234,
		Target: model.ScanTarget{
			GitHubBranch: branch,
			CommitID:     "xyz098",
		},
		Sources: []*model.PackageSource{
			{
				Source: "Gemfile.lock",
				Packages: []*model.Package{
					{
						Type:            model.PkgRubyGems,
						Name:            "orange",
						Version:         "1.1",
						Vulnerabilities: []string{"CVE-2999-0002"},
					},
				},
			},
		},
	}
	newReport := &model.ScanReport{
		ReportID:  "abc",
		ScannedAt: 2234,
		Target: model.ScanTarget{
			GitHubBranch: branch,
			CommitID:     "abc123",
		},
		Sources: []*model.PackageSource{
			{
				Source: "Gemfile.lock",
				Packages: []*model.Package{
					{
						Type:            model.PkgRubyGems,
						Name:            "blue",
						Version:         "1.1",
						Vulnerabilities: []string{"CVE-2999-0001"},
					},
				},
			},
		},
	}

	// Insert test data
	require.NoError(t, mock.db.InsertScanReport(oldReport))
	require.NoError(t, mock.db.InsertScanReport(newReport))
	inserted, err := mock.db.InsertRepo(&model.Repository{
		GitHubRepo:    branch.GitHubRepo,
		DefaultBranch: "main",
	})
	require.NoError(t, err)
	require.True(t, inserted)
	require.NoError(t, mock.db.UpdateBranch(&model.Branch{
		GitHubBranch: branch,
		ReportSummary: model.ScanReportSummary{
			ReportID: "ebc",
		},
	}))

	calledCreateIssueCommentMock := false
	mock.githubapp.CreateIssueCommentMock = func(repo *model.GitHubRepo, prID int, body string) error {
		calledCreateIssueCommentMock = true
		assert.Equal(t, "clock", repo.Owner)
		assert.Equal(t, "tower", repo.RepoName)
		assert.Equal(t, 666, prID)
		assert.Contains(t, body, "🚨")
		assert.Contains(t, body, "✅")
		assert.NotContains(t, body, "⚠️")
		return nil
	}
	calledUpdateCheckRunMock := false
	mock.githubapp.UpdateCheckRunMock = func(repo *model.GitHubRepo, checkID int64, opt *github.UpdateCheckRunOptions) error {
		calledUpdateCheckRunMock = true
		return nil
	}
	require.NoError(t, uc.FeedbackScanResult(req))
	assert.True(t, calledCreateIssueCommentMock)
	assert.False(t, calledUpdateCheckRunMock)
}

func TestFeedbackScanResultPullReqComment(t *testing.T) {
	srcBranch := model.GitHubBranch{
		GitHubRepo: model.GitHubRepo{
			Owner:    "clock",
			RepoName: "tower",
		},
		Branch: "m-mizutani:pr",
	}

	originPkg := &model.Package{
		Type:            model.PkgRubyGems,
		Name:            "origin",
		Version:         "1.1",
		Vulnerabilities: []string{"CVE-2999-0002"},
	}

	firstReport := &model.ScanReport{
		ReportID:  "first-report-id",
		ScannedAt: 1234,
		Target: model.ScanTarget{
			GitHubBranch: srcBranch,
			CommitID:     "xyz098",
		},
		Sources: []*model.PackageSource{
			{
				Source: "Gemfile.lock",
				Packages: []*model.Package{
					{
						Type:            model.PkgRubyGems,
						Name:            "orange",
						Version:         "1.1",
						Vulnerabilities: []string{"CVE-2999-0002"},
					},
					originPkg,
				},
			},
		},
	}
	secondReport := &model.ScanReport{
		ReportID:  "second-report-id",
		ScannedAt: 2234,
		Target: model.ScanTarget{
			GitHubBranch: srcBranch,
			CommitID:     "abc123",
		},
		Sources: []*model.PackageSource{
			{
				Source: "Gemfile.lock",
				Packages: []*model.Package{
					{
						Type:            model.PkgRubyGems,
						Name:            "orange",
						Version:         "1.1",
						Vulnerabilities: []string{"CVE-2999-0002"},
					},
					originPkg,
				},
			},
		},
	}
	thirdReport := &model.ScanReport{
		ReportID:  "third-report-id",
		ScannedAt: 2234,
		Target: model.ScanTarget{
			GitHubBranch: srcBranch,
			CommitID:     "ppp123",
		},
		Sources: []*model.PackageSource{
			{
				Source: "Gemfile.lock",
				Packages: []*model.Package{
					{
						Type:            model.PkgRubyGems,
						Name:            "blue",
						Version:         "1.1",
						Vulnerabilities: []string{"CVE-2999-0001"},
					},
					originPkg,
				},
			},
		},
	}

	setup := func(t *testing.T) (interfaces.Usecases, *mockSet) {
		uc, mock := setupFeedbackScanResult(t)

		dstBranch := model.GitHubBranch{
			GitHubRepo: model.GitHubRepo{
				Owner:    "clock",
				RepoName: "tower",
			},
			Branch: "main",
		}

		dstReport := &model.ScanReport{
			ReportID:  "dst-report-id",
			ScannedAt: 1234,
			Target: model.ScanTarget{
				GitHubBranch: srcBranch,
				CommitID:     "xyz098",
			},
			Sources: []*model.PackageSource{
				{
					Source:   "Gemfile.lock",
					Packages: []*model.Package{originPkg},
				},
			},
		}

		// Insert test data
		require.NoError(t, mock.db.InsertScanReport(dstReport))
		inserted, err := mock.db.InsertRepo(&model.Repository{
			GitHubRepo:    dstBranch.GitHubRepo,
			DefaultBranch: "main",
		})
		require.NoError(t, err)
		require.True(t, inserted)
		require.NoError(t, mock.db.UpdateBranch(&model.Branch{
			GitHubBranch: dstBranch,
			ReportSummary: model.ScanReportSummary{
				ReportID: "dst-report-id",
			},
		}))

		// Do not check UpdateCheckRun
		mock.githubapp.UpdateCheckRunMock = func(repo *model.GitHubRepo, checkID int64, opt *github.UpdateCheckRunOptions) error { return nil }
		return uc, mock
	}

	t.Run("comment new vuln by first report", func(t *testing.T) {
		uc, mock := setup(t)
		require.NoError(t, mock.db.InsertScanReport(firstReport))

		calledCreateIssueCommentMock := false
		mock.githubapp.CreateIssueCommentMock = func(repo *model.GitHubRepo, prID int, body string) error {
			calledCreateIssueCommentMock = true
			assert.Equal(t, "clock", repo.Owner)
			assert.Equal(t, "tower", repo.RepoName)
			assert.Equal(t, 666, prID)
			assert.Contains(t, body, "🚨")
			assert.NotContains(t, body, "✅")
			assert.NotContains(t, body, "origin")
			return nil
		}

		req := &model.FeedbackRequest{
			ReportID:  "first-report-id",
			InstallID: 123,
			Options: model.FeedbackOptions{
				PullReqID:     model.Int(666),
				PullReqBranch: "main",
			},
		}

		require.NoError(t, uc.FeedbackScanResult(req))
		assert.True(t, calledCreateIssueCommentMock)
	})

	t.Run("do not comment by second report", func(t *testing.T) {
		uc, mock := setup(t)
		require.NoError(t, mock.db.InsertScanReport(firstReport))
		require.NoError(t, mock.db.InsertScanReport(secondReport))

		calledCreateIssueCommentMock := false
		mock.githubapp.CreateIssueCommentMock = func(repo *model.GitHubRepo, prID int, body string) error {
			calledCreateIssueCommentMock = true
			return nil
		}

		req := &model.FeedbackRequest{
			ReportID:  "second-report-id",
			InstallID: 123,
			Options: model.FeedbackOptions{
				PullReqID:     model.Int(666),
				PullReqBranch: "main",
			},
		}

		require.NoError(t, uc.FeedbackScanResult(req))
		assert.False(t, calledCreateIssueCommentMock)
	})

	t.Run("comment fix vuln by third report", func(t *testing.T) {
		uc, mock := setup(t)
		require.NoError(t, mock.db.InsertScanReport(firstReport))
		require.NoError(t, mock.db.InsertScanReport(secondReport))
		require.NoError(t, mock.db.InsertScanReport(thirdReport))

		calledCreateIssueCommentMock := false
		mock.githubapp.CreateIssueCommentMock = func(repo *model.GitHubRepo, prID int, body string) error {
			calledCreateIssueCommentMock = true
			assert.Equal(t, "clock", repo.Owner)
			assert.Equal(t, "tower", repo.RepoName)
			assert.Equal(t, 666, prID)
			assert.Contains(t, body, "🚨")
			assert.Contains(t, body, "✅")
			assert.NotContains(t, body, "origin")
			return nil
		}

		req := &model.FeedbackRequest{
			ReportID:  "third-report-id",
			InstallID: 123,
			Options: model.FeedbackOptions{
				PullReqID:     model.Int(666),
				PullReqBranch: "main",
			},
		}

		require.NoError(t, uc.FeedbackScanResult(req))
		assert.True(t, calledCreateIssueCommentMock)
	})
}
