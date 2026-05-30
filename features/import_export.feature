Feature: Import / Export — OpenAPI in, cURL out

  Helena imports OpenAPI 3 and Swagger 2 specs into its own
  collection model, and exports any request as a cURL command. These
  scenarios drive both halves end-to-end through the storage +
  session layers, asserting tree shape and shell-safe rendering.

  Scenario: OpenAPI 3 spec imports into expected tree shape
    Given an OpenAPI 3 spec:
      """
      openapi: 3.0.0
      info:
        title: Sample
      servers:
        - url: https://api.example.com/v1
      paths:
        /users:
          get:
            summary: List users
            tags: [users]
          post:
            summary: Create user
            tags: [users]
            requestBody:
              content:
                application/json:
                  example:
                    name: Alice
        /health:
          get:
            summary: Health
      """
    When I import the spec
    Then the collection has a folder "users"
    And the collection has a request "users/List users"
    And the collection has a request "users/Create user"
    And the collection has a request "Health"
    And the collection has an environment variable "base_url" set to "https://api.example.com/v1"

  Scenario: Imported POST carries the JSON body example
    Given an OpenAPI 3 spec:
      """
      openapi: 3.0.0
      info:
        title: Sample
      servers:
        - url: https://api.example.com
      paths:
        /users:
          post:
            summary: Create user
            tags: [users]
            requestBody:
              content:
                application/json:
                  example:
                    name: Alice
      """
    When I import the spec
    Then the request "users/Create user" has method "POST"
    And the request "users/Create user" body contains "Alice"

  Scenario: Swagger 2 spec converts to the same shape
    Given a Swagger 2 spec:
      """
      swagger: "2.0"
      info:
        title: Legacy
      host: api.legacy.com
      basePath: /v1
      schemes:
        - https
      paths:
        /ping:
          get:
            summary: Ping
            tags:
              - ops
      """
    When I import the spec
    Then the collection has a folder "ops"
    And the collection has a request "ops/Ping"
    And the collection has an environment variable "base_url" set to "https://api.legacy.com/v1"

  Scenario: Imported request renders to cURL preserving method, URL, and body
    Given an OpenAPI 3 spec:
      """
      openapi: 3.0.0
      info:
        title: Sample
      servers:
        - url: https://api.example.com
      paths:
        /users:
          post:
            summary: Create user
            requestBody:
              content:
                application/json:
                  example:
                    name: Bob
      """
    When I import the spec
    And I render "Create user" as cURL
    Then the cURL contains "curl -X POST"
    And the cURL contains "https://api.example.com/users"
    And the cURL contains "Content-Type"
    And the cURL contains "Bob"
