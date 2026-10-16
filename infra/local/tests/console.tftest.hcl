mock_provider "kubernetes" {}

mock_provider "helm" {}

variables {
  image_digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}

run "console_is_plain_http_on_localhost" {
  command = plan

  assert {
    condition     = output.console_url == "http://console.localhost"
    error_message = "local console must be served over http on localhost"
  }
}
