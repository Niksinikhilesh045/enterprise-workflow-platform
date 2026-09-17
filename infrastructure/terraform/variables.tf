variable "project_name" {
  description = "Prefix used for AWS resource names."
  type        = string
  default     = "enterprise-workflow"
}

variable "aws_region" {
  description = "AWS region for regional resources."
  type        = string
  default     = "ap-south-1"
}

variable "image_tag" {
  description = "Container image tag deployed from the project ECR repository."
  type        = string
  default     = "latest"
}

variable "mongo_uri_secret_arn" {
  description = "Secrets Manager ARN containing the MongoDB connection URI."
  type        = string
}

variable "kafka_brokers_secret_arn" {
  description = "Secrets Manager ARN containing a comma-separated Kafka broker list."
  type        = string
}

variable "auth_secret_arn" {
  description = "Secrets Manager ARN containing the 32+ character API signing secret."
  type        = string
}

variable "api_desired_count" {
  description = "Number of API Fargate tasks."
  type        = number
  default     = 2
}

variable "worker_desired_count" {
  description = "Number of tasks for each background worker service."
  type        = number
  default     = 1
}
