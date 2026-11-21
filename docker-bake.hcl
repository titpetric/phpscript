# latest follows main and is unstable. A release passes TAG in the environment.
variable "TAG" {
  default = "latest"
}

group "default" {
  targets = ["phpscript-amd64", "phpscript-arm64"]
}

target "phpscript-amd64" {
  context    = "."
  dockerfile = "docker/Dockerfile.build"
  tags       = ["titpetric/phpscript:${TAG}-amd64"]
  platforms  = ["linux/amd64"]
  attest     = ["type=sbom"]
}

target "phpscript-arm64" {
  context    = "."
  dockerfile = "docker/Dockerfile.build"
  tags       = ["titpetric/phpscript:${TAG}-arm64"]
  platforms  = ["linux/arm64"]
  attest     = ["type=sbom"]
}
