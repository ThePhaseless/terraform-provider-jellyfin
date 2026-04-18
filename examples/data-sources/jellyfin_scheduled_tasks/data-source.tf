data "jellyfin_scheduled_tasks" "all" {}

output "task_ids" {
  value = { for t in data.jellyfin_scheduled_tasks.all.tasks : t.name => t.id }
}
