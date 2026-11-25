Rails.application.routes.draw do
  root to: proc { [200, { "content-type" => "text/plain" }, ["E2E-RAILS-OK uid=#{Process.uid}\n"]] }
end
