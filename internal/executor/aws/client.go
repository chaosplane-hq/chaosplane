package aws

import (
	"context"
	"fmt"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/rds"
)

type AWSClient struct {
	EC2 *ec2.Client
	RDS *rds.Client
	ECS *ecs.Client
}

func NewAWSClient(cfg AWSConfig) (*AWSClient, error) {
	opts := []func(*awscfg.LoadOptions) error{
		awscfg.WithRegion(cfg.Region),
	}

	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		opts = append(opts, awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, cfg.SessionToken),
		))
	}

	sdkCfg, err := awscfg.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return &AWSClient{
		EC2: ec2.NewFromConfig(sdkCfg),
		RDS: rds.NewFromConfig(sdkCfg),
		ECS: ecs.NewFromConfig(sdkCfg),
	}, nil
}
