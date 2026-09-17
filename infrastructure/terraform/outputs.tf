output "api_url" {
  description = "Public API base URL for the demo deployment."
  value       = "http://${aws_lb.api.dns_name}"
}

output "web_url" {
  description = "CloudFront URL for the React web application."
  value       = "https://${aws_cloudfront_distribution.web.domain_name}"
}

output "web_bucket" {
  description = "Private S3 bucket receiving the React production build."
  value       = aws_s3_bucket.web.id
}

output "ecr_repository_url" {
  description = "ECR repository that stores the shared Go service image."
  value       = aws_ecr_repository.app.repository_url
}

output "ecs_cluster_name" {
  description = "ECS cluster hosting API and worker services."
  value       = aws_ecs_cluster.main.name
}
