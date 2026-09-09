// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	action "github.com/hashicorp/terraform-plugin-framework/action"
	datasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	resource "github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestProvider_RegisteredSurface pins the exact set of full type names the
// provider registers, per ADR 0001. Adding or removing any component must
// update the corresponding want list here; the test exists to make silent
// surface drift impossible.
func TestProvider_RegisteredSurface(t *testing.T) {
	p := newTestProvider()
	ctx := context.Background()

	gotResources := registeredResourceNames(t, ctx, p)
	gotDataSources := registeredDataSourceNames(t, ctx, p)
	gotActions := registeredActionNames(t, ctx, p)

	wantResources := []string{
		"pve_node",
		"pve_node_network_linux_bridge",
		"pve_node_network_linux_bond",
		"pve_node_network_vlan",
		"pve_node_disk_zfs",
		"pve_user",
		"pve_user_token",
		"pve_group",
		"pve_role",
		"pve_acl",
		"pve_realm_ldap",
		"pve_realm_ad",
		"pve_realm_openid",
		"pve_realm_sync_job",
		"pve_cluster_node",
		"pve_cluster_options",
		"pve_ha_group",
		"pve_ha_resource",
		"pve_ha_rule",
		"pve_cluster_firewall_options",
		"pve_node_firewall_options",
		"pve_guest_firewall_options",
		"pve_sdn_firewall_options",
		"pve_firewall_alias",
		"pve_firewall_ipset",
		"pve_firewall_security_group",
		"pve_cluster_firewall_rules",
		"pve_node_firewall_rules",
		"pve_guest_firewall_rules",
		"pve_security_group_firewall_rules",
		"pve_vnet_firewall_rules",
		"pve_backup_job",
		"pve_replication",
		"pve_metrics_server",
		"pve_notification_endpoint_sendmail",
		"pve_notification_endpoint_goty",
		"pve_notification_endpoint_smtp",
		"pve_notification_endpoint_webhook",
		"pve_notification_matcher",
		"pve_pool",
		"pve_storage_nfs",
		"pve_storage_cifs",
		"pve_storage_iscsi",
		"pve_storage_iscsidirect",
		"pve_storage_lvm",
		"pve_storage_lvmthin",
		"pve_storage_zfspool",
		"pve_storage_directory",
		"pve_storage_pbs",
		"pve_storage_cephfs",
		"pve_storage_rbd",
		"pve_file",
		"pve_download_file",
		"pve_node_hosts",
		"pve_node_disk_lvmthin",
		"pve_node_disk_directory",
		"pve_ceph_pool",
		"pve_ceph_osd",
		"pve_ceph_mon",
		"pve_acme_account",
		"pve_acme_dns_plugin",
		"pve_hardware_mapping_pci",
		"pve_hardware_mapping_usb",
		"pve_mapping_dir",
		"pve_custom_cpu_model",
		"pve_node_certificate",
		"pve_acme_certificate",
		"pve_apt_standard_repository",
		"pve_vm",
		"pve_vm_snapshot",
		"pve_container",
		"pve_container_snapshot",
		"pve_sdn_zone_simple",
		"pve_sdn_zone_vlan",
		"pve_sdn_zone_qinq",
		"pve_sdn_zone_vxlan",
		"pve_sdn_zone_evpn",
		"pve_sdn_vnet",
		"pve_sdn_subnet",
		"pve_sdn_controller",
		"pve_sdn_dns",
		"pve_sdn_ipam",
		"pve_sdn_prefix_list",
		"pve_sdn_route_map",
		"pve_sdn_fabric_ospf",
		"pve_sdn_fabric_openfabric",
	}
	wantDataSources := []string{
		"pve_nodes",
		"pve_node_status",
		"pve_node_disks",
		"pve_node_network_interfaces",
		"pve_user",
		"pve_user_token",
		"pve_group",
		"pve_role",
		"pve_acl",
		"pve_permissions",
		"pve_realm_ldap",
		"pve_realm_ad",
		"pve_realm_openid",
		"pve_realms",
		"pve_realm_sync_job",
		"pve_cluster_resources",
		"pve_cluster_status",
		"pve_tasks",
		"pve_version",
		"pve_cluster_node",
		"pve_cluster_options",
		"pve_ha_status",
		"pve_ha_group",
		"pve_ha_resource",
		"pve_ha_rule",
		"pve_cluster_firewall_options",
		"pve_node_firewall_options",
		"pve_guest_firewall_options",
		"pve_sdn_firewall_options",
		"pve_firewall_alias",
		"pve_firewall_ipset",
		"pve_firewall_security_group",
		"pve_backup_job",
		"pve_backup_jobs",
		"pve_replication",
		"pve_node_replications",
		"pve_metrics_server",
		"pve_notification_endpoint_sendmail",
		"pve_notification_endpoint_goty",
		"pve_notification_endpoint_smtp",
		"pve_notification_endpoint_webhook",
		"pve_notification_matcher",
		"pve_notification_targets",
		"pve_pool",
		"pve_storage_nfs",
		"pve_storage_cifs",
		"pve_storage_iscsi",
		"pve_storage_iscsidirect",
		"pve_storage_lvm",
		"pve_storage_lvmthin",
		"pve_storage_zfspool",
		"pve_storage_directory",
		"pve_storage_pbs",
		"pve_storage_cephfs",
		"pve_storage_rbd",
		"pve_node_storages",
		"pve_storage_files",
		"pve_file",
		"pve_download_file",
		"pve_node",
		"pve_node_hosts",
		"pve_node_disk_lvmthin",
		"pve_node_disk_directory",
		"pve_node_services",
		"pve_node_apt_repositories",
		"pve_apt_standard_repository",
		"pve_node_tasks",
		"pve_node_pci_devices",
		"pve_node_usb_devices",
		"pve_node_capabilities",
		"pve_ceph_status",
		"pve_ceph_pool",
		"pve_ceph_osd",
		"pve_ceph_mon",
		"pve_acme_account",
		"pve_acme_plugins",
		"pve_acme_dns_plugin",
		"pve_hardware_mapping_pci",
		"pve_hardware_mapping_usb",
		"pve_mapping_dir",
		"pve_custom_cpu_model",
		"pve_node_certificate",
		"pve_acme_certificate",
		"pve_vms",
		"pve_vm",
		"pve_vm_snapshot",
		"pve_containers",
		"pve_container",
		"pve_container_snapshot",
		"pve_sdn_zone_simple",
		"pve_sdn_zone_vlan",
		"pve_sdn_zone_qinq",
		"pve_sdn_zone_vxlan",
		"pve_sdn_zone_evpn",
		"pve_sdn_vnet",
		"pve_sdn_subnet",
		"pve_sdn_controller",
		"pve_sdn_dns",
		"pve_sdn_ipam",
		"pve_sdn_prefix_list",
		"pve_sdn_route_map",
		"pve_sdn_fabric_ospf",
		"pve_sdn_fabric_openfabric",
		"pve_appliances",
		"pve_vm_agent_info",
		"pve_node_subscription",
	}
	wantActions := []string{
		"pve_realm_sync",
		"pve_ha_arm",
		"pve_notification_test",
		"pve_storage_prune_backups",
		"pve_node_reboot",
		"pve_node_shutdown",
		"pve_node_service",
		"pve_node_apt_update",
		"pve_node_wakeonlan",
		"pve_vm_reboot",
		"pve_vm_suspend",
		"pve_vm_resume",
		"pve_vm_reset",
		"pve_vm_migrate",
		"pve_vm_snapshot_rollback",
		"pve_container_reboot",
		"pve_container_suspend",
		"pve_container_resume",
		"pve_container_migrate",
		"pve_container_snapshot_rollback",
		"pve_sdn_apply",
		"pve_sdn_rollback",
		"pve_user_tfa_unlock",
		"pve_ha_resource_migrate",
		"pve_ha_resource_relocate",
		"pve_replication_schedule_now",
		"pve_backup_run",
		"pve_subscription_refresh",
		"pve_node_disk_initgpt",
		"pve_task_cancel",
		"pve_aplinfo_update",
		"pve_storage_oci_pull",
		"pve_node_execute",
		"pve_node_start_all",
		"pve_node_stop_all",
		"pve_node_suspend_all",
		"pve_node_migrate_all",
		"pve_guest_bulk_start",
		"pve_guest_bulk_shutdown",
		"pve_guest_bulk_suspend",
		"pve_guest_bulk_migrate",
	}

	assertSameSet(t, "resources", wantResources, gotResources)
	assertSameSet(t, "data sources", wantDataSources, gotDataSources)
	assertSameSet(t, "actions", wantActions, gotActions)
}

// registeredResourceNames collects Metadata type names for every registered
// resource.
func registeredResourceNames(t *testing.T, ctx context.Context, p *PveProvider) []string {
	t.Helper()
	var names []string
	for _, f := range p.Resources(ctx) {
		resp := &resource.MetadataResponse{}
		f().Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, resp)
		names = append(names, resp.TypeName)
	}
	return names
}

// registeredDataSourceNames collects Metadata type names for every
// registered data source.
func registeredDataSourceNames(t *testing.T, ctx context.Context, p *PveProvider) []string {
	t.Helper()
	var names []string
	for _, f := range p.DataSources(ctx) {
		resp := &datasource.MetadataResponse{}
		f().Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, resp)
		names = append(names, resp.TypeName)
	}
	return names
}

// registeredActionNames collects Metadata type names for every registered
// action.
func registeredActionNames(t *testing.T, ctx context.Context, p *PveProvider) []string {
	t.Helper()
	var names []string
	for _, f := range p.Actions(ctx) {
		resp := &action.MetadataResponse{}
		f().Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, resp)
		names = append(names, resp.TypeName)
	}
	return names
}

// assertSameSet fails with a precise diff when got differs from want as a
// set, including duplicates in got.
func assertSameSet(t *testing.T, kind string, want, got []string) {
	t.Helper()
	wantSet := make(map[string]int, len(want))
	for _, w := range want {
		wantSet[w]++
	}
	gotSet := make(map[string]int, len(got))
	for _, g := range got {
		gotSet[g]++
	}
	for name, n := range wantSet {
		if gotSet[name] < n {
			t.Errorf("%s: %s registered fewer times than expected (want %d, got %d)", kind, name, n, gotSet[name])
		}
	}
	for name, n := range gotSet {
		if wantSet[name] < n {
			t.Errorf("%s: unexpected registration %s (or duplicate): want %d, got %d", kind, name, wantSet[name], n)
		}
	}
	if len(want) != len(got) {
		t.Errorf("%s: count mismatch: want %d, got %d", kind, len(want), len(got))
	}
}
