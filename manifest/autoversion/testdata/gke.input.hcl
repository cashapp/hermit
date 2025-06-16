description = "Google Cloud SDK gke-gcloud-auth-plugin component"
binaries = ["gke-gcloud-auth-plugin"]
test = "gke-gcloud-auth-plugin --help"

darwin {
  source = "https://dl.google.com/dl/cloudsdk/channels/rapid/components/google-cloud-sdk-gke-gcloud-auth-plugin-darwin-${xarch}-${build_number}.tar.gz"
}

linux {
  source = "https://dl.google.com/dl/cloudsdk/channels/rapid/components/google-cloud-sdk-gke-gcloud-auth-plugin-linux-${arch}-${build_number}.tar.gz"
}

version "0.5.10" {
  auto-version {
    json {
      url = "https://dl.google.com/dl/cloudsdk/channels/rapid/components-2.json"
      path = "components.#(id==\"gke-gcloud-auth-plugin-linux-x86_64\").version.version_string"
    }
    version-pattern = "(.*)"
  }
  
  darwin {
    vars {
      build_number = "${json:components.#(id==\"gke-gcloud-auth-plugin-darwin-x86_64\").version.build_number}"
    }
    sha256 = "${json:components.#(id==\"gke-gcloud-auth-plugin-darwin-x86_64\").data.checksum}"
    
    platform "arm64" {
      vars {
        build_number = "${json:components.#(id==\"gke-gcloud-auth-plugin-darwin-arm\").version.build_number}"
      }
      sha256 = "${json:components.#(id==\"gke-gcloud-auth-plugin-darwin-arm\").data.checksum}"
    }
  }
  
  linux {
    vars {
      build_number = "${json:components.#(id==\"gke-gcloud-auth-plugin-linux-x86_64\").version.build_number}"
    }
    sha256 = "${json:components.#(id==\"gke-gcloud-auth-plugin-linux-x86_64\").data.checksum}"
  }
}