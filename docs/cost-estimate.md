# Production cost estimate (AWS, ap-southeast-1)

Leader estimate, 2026-10-18, for the go-live phase (D-31, T-023). **Approximate list prices from the leader's knowledge, ±20 %, not read from the AWS price list;** the OpenTofu `plan` plus the AWS Pricing Calculator replace these numbers before anything is provisioned. USD per month, 730 hours, no free tier or credits applied.

| Item | Lean (first users, no failover) | Growth (real traffic, failover) |
|---|---|---|
| Fargate API, 0.5 vCPU / 1 GB | 1 task: 22 | 2 tasks: 45 |
| RDS MySQL 8.4 db.t4g.micro + 20 GB gp3 | single-AZ: 17 | Multi-AZ: 34 |
| ElastiCache Valkey cache.t4g.micro (D-23) | 1 node: 12 | 2 nodes: 24 |
| Application Load Balancer (incl. its 2 public IPv4) | 26 | 26 |
| Public IPv4 for the Fargate tasks (no NAT, see below) | 4 | 7 |
| S3 photos (50 GB / 200 GB) | 2 | 5 |
| Secrets Manager (about 8 secrets) | 3 | 3 |
| CloudWatch logs, alarms, metrics (T-024) | 4 | 8 |
| Route 53 zone + uptime check | 1 | 1 |
| CloudFront (inside the 1 TB / 10 M request free tier) | 0 | 0 to 5 |
| SES + SNS (about 2 to 5 thousand mails) | 1 | 2 |
| ECR, state bucket, misc | 1 | 1 |
| Domain (.com, per month) | 1 | 1 |
| **Total** | **about 95** (about 1,150 a year) | **about 160** (about 1,900 a year) |

## What is not in the table
- **NAT gateway: +45 plus data.** Fargate tasks in private subnets need a NAT gateway (about 45) or four interface VPC endpoints (about 8 each: ECR api, ECR dkr, Logs, Secrets Manager). Recommended: RDS and ElastiCache in private subnets with no internet route, API tasks in public subnets whose security group accepts the load balancer only. This is a security-versus-cost choice for the owner; decide when T-023 is specified.
- **AWS WAF for the edge rate limits (T-031): +8 to 10** (web ACL 5, one rule 1 each, requests 0.60 per million).
- Data transfer out is covered by the CloudFront free tier at this size; a viral link would show up as CloudFront and S3 lines first.
- Free: ACM certificate, GitHub Actions on the public repo, Trivy, Google sign-in.
- New AWS accounts may receive sign-up credits (up to about 200 USD in 2025 for the first six months); check at account creation, then re-run the numbers.

## Levers, biggest first
1. No NAT gateway (above): saves about 45.
2. Region `us-east-1` instead of Singapore: about 15 to 20 % cheaper, but about 200 ms further from Vietnam users.
3. ARM (Graviton) Fargate: about 20 % off the Fargate line, needs an arm64 image build.
4. ElastiCache Serverless (Valkey): about 6 minimum, saves about 6 versus a node, price grows with use.
5. RDS single-AZ until the first real users: already assumed in the lean column; backups stay on (7 days, free up to the DB size).

## Budget guard
Create an AWS Budget before the first `apply`: lean 120 per month, growth 200, alerts at 50 %, 80 % and 100 % of forecast to the owner's email (T-023 acceptance criterion).
