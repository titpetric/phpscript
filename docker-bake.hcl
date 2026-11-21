# latest follows main and is unstable. A release passes TAG in the environment.
variable "TAG" {
  default = "latest"
}

group "default" {
  targets = ["phpscript"]
}

target "phpscript" {
  context    = "."
  dockerfile = "docker/Dockerfile.build"
  tags       = ["titpetric/phpscript:${TAG}"]
  platforms  = ["linux/amd64", "linux/arm64"]
}
