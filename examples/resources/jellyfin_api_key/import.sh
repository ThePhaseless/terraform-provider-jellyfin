# An API key is imported by its app name, or by its access token when several keys share the app name.
terraform import jellyfin_api_key.example "my-application"
terraform import jellyfin_api_key.example <access-token>
