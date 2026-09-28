resource "jellyfin_scheduled_task" "example" {
  key = "RefreshLibrary"

  triggers = [
    {
      type           = "IntervalTrigger"
      interval_ticks = 432000000000
    }
  ]
}
