// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// Phase 5 acceptance scenarios (ROADMAP.md): storage CRUD round-trip and
// pool lifecycle with import + no-op re-plan, and the file download-url
// flow. Writes are contained: a scratch dir-backed storage under /tmp on
// the node, pool metadata, and one ISO fetched from an HTTP server on the
// test machine onto the `local` storage (deleted afterwards). The
// multipart upload flow is a documented engine gap and not covered.

func testAccAPIGone(t *testing.T, what, path string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		_, err := testAccClient(t).DoRequest(context.Background(), "GET", path, nil)
		if pveclient.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("%s still exists", what)
	}
}

func TestAccPhase5PoolLifecycle(t *testing.T) {
	poolid := fmt.Sprintf("tfacc-pool-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	config := func(comment string) string {
		return fmt.Sprintf(`
resource "pve_pool" "test" {
  poolid  = %q
  comment = %q
}
`, poolid, comment)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccAPIGone(t, "pool "+poolid, "/pools/"+poolid),
		Steps: []resource.TestStep{
			{
				Config: config("one"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_pool.test", tfjsonpath.New("id"), knownvalue.StringExact(poolid)),
					statecheck.ExpectKnownValue("pve_pool.test", tfjsonpath.New("comment"), knownvalue.StringExact("one")),
				},
			},
			{
				Config: config("two"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_pool.test", tfjsonpath.New("comment"), knownvalue.StringExact("two")),
				},
			},
			{
				ResourceName:      "pve_pool.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccPhase5StorageDirLifecycle(t *testing.T) {
	sid := fmt.Sprintf("tfaccdir%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlpha))
	path := "/tmp/" + sid
	config := func(content string) string {
		return fmt.Sprintf(`
resource "pve_storage" "test" {
  storage = %q
  type    = "dir"
  path    = %q
  content = [%s]
  mkdir   = true
}
`, sid, path, content)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccAPIGone(t, "storage "+sid, "/storage/"+sid),
		Steps: []resource.TestStep{
			{
				Config: config(`"snippets"`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pve_storage.test", tfjsonpath.New("id"), knownvalue.StringExact(sid)),
					statecheck.ExpectKnownValue("pve_storage.test", tfjsonpath.New("type"), knownvalue.StringExact("dir")),
				},
			},
			{
				Config:            config(`"iso", "snippets"`),
				ConfigStateChecks: []statecheck.StateCheck{},
			},
			{
				ResourceName:      "pve_storage.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// testAccServeFile serves dir over HTTP on all interfaces and returns the
// port; the PVE node fetches from this machine during the test.
func testAccServeFile(t *testing.T, dir string) int {
	t.Helper()
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(dir, filepath.Clean(r.URL.Path)))
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address %T is not TCP", ln.Addr())
	}
	return addr.Port
}

func TestAccPhase5FileDownloadURL(t *testing.T) {
	node := testAccNode(t)
	name := fmt.Sprintf("tfacc-%s.iso", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("tfacc placeholder iso\n"), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}
	port := testAccServeFile(t, dir)

	config := fmt.Sprintf(`
data "pve_storage" "lb" {
  node       = %q
  storage    = "local-btrfs"
  depends_on = [terraform_data.trigger]
}

action "pve_file_download_url" "dl" {
  config {
    node     = %q
    storage  = "local-btrfs"
    content  = "iso"
    filename = %q
    url      = "http://10.0.10.110:%d/%s"
  }
}

resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.pve_file_download_url.dl]
    }
  }
}

output "downloaded" {
  value = anytrue([for f in data.pve_storage.lb.content : f.volid == "local-btrfs:iso/%s"])
}
`, node, node, name, port, name, name)

	volid := "local-btrfs:iso/" + name
	deleteFile := func(s *terraform.State) error {
		_, err := testAccClient(t).DoRequest(context.Background(), "DELETE", "/nodes/"+node+"/storage/local-btrfs/content/"+volid, nil)
		if err != nil && !pveclient.IsNotFound(err) {
			return err
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             deleteFile,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckOutput("downloaded", "true"),
				),
			},
		},
	})
}
