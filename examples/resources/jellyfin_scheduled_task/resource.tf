# Configure the "Scan Media Library" task to run every 12 hours
resource "jellyfin_scheduled_task" "scan_library" {
  id = "7738148ffcd07979c7ceb148e06b3aed"

  triggers_json = jsonencode([
    {
      Type          = "IntervalTrigger"
      IntervalTicks = 432000000000
    }
  ])
}
