packer {
  required_plugins {
    libvirt = {
      version = ">= 0.0.1"
      source  = "github.com/hashicorp/libvirt"
    }
  }
}

build {
  sources = ["source.libvirt.example"]
}
