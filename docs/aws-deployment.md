# AWS Deployment Blueprint

This repository includes a validated Terraform blueprint for deploying the platform to AWS. The infrastructure code is intentionally separate from actual account deployment: CI validates syntax/provider compatibility, while `terraform apply` requires an explicitly configured AWS account and external secrets.

## Architecture

```text
React build
   |
   v
Private S3 bucket <--- CloudFront (HTTPS)

Internet
   |
   v
Application Load Balancer (HTTP demo endpoint)
   |
   v
ECS Fargate API (2 tasks by default)
   |                  \
   |                   \--> MongoDB endpoint from Secrets Manager
   |
ECS Fargate outbox relay ---> Kafka endpoint from Secrets Manager
   |
ECS Fargate event worker ---> Kafka + MongoDB

All Go processes use the same ECR image. ECS overrides the container command for background workers.
```

## Secret handling

Terraform accepts three Secrets Manager ARNs:

- `mongo_uri_secret_arn`
- `kafka_brokers_secret_arn`
- `auth_secret_arn`

The secret values themselves are not stored in this repository or passed as Terraform variables. The ECS execution role is granted `secretsmanager:GetSecretValue` only for those ARNs.

## Validate locally

```bash
cd infrastructure/terraform
terraform fmt -check -recursive
terraform init -backend=false
terraform validate
```

The same validation runs in GitHub Actions.

## Build the shared service image

```bash
docker build -t enterprise-workflow-api ./services/api
```

The image contains:

- `/app/api`
- `/app/outbox-relay`
- `/app/event-worker`
- `/app/token`

The default command runs the API. ECS task definitions override the command for the workers.

## Before applying

1. Create MongoDB, Kafka, and auth secrets in AWS Secrets Manager.
2. Copy `terraform.tfvars.example` to a local `terraform.tfvars` and replace only the example ARNs and deployment settings.
3. Build and push an image to the ECR repository created by Terraform (or bootstrap ECR first in a separate state/workspace).
4. Use an immutable image tag such as the Git commit SHA.
5. Run `terraform plan` and review the cost/security impact before `terraform apply`.
6. Build `apps/web` with the deployed API URL and sync `dist/` to the output S3 bucket.
7. Invalidate the CloudFront cache after web deployment.

## Scope and trade-offs

For a portfolio/demo environment, Fargate tasks receive public IPs while inbound access is restricted by security groups; only the API receives inbound traffic from the ALB. This avoids NAT Gateway cost. A production deployment should move application tasks into private subnets, add NAT/VPC endpoints as appropriate, terminate TLS at the ALB using ACM, and add WAF/monitoring/backup policies based on organizational requirements.

The blueprint deliberately treats MongoDB and Kafka as external endpoints. This keeps the application portable between MongoDB Atlas/Amazon-compatible choices and Kafka providers such as Amazon MSK without embedding vendor-specific database or broker lifecycle management into this project.
