resource "jellyfin_scheduled_task" "example" {
  task_id = "7738148ffcd07979c7ceb148e06b3aed"

  triggers = [
    {
      type           = "IntervalTrigger"
      interval_ticks = 432000000000
    }
  ]
}
