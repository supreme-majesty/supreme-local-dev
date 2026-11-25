# Proves Rails detection (bin/rails server -b 0.0.0.0 -p 3000), bundle install and routing.
# Only Action Controller is loaded, so the app needs no database.
require_relative "boot"
require "rails"
require "action_controller/railtie"

Bundler.require(*Rails.groups)

class E2eRails < Rails::Application
  config.load_defaults 8.0
  config.eager_load = false
  config.logger = Logger.new($stdout)
  config.secret_key_base = "e2e-not-secret"
end
