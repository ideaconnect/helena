Feature: Send a request through the internals

  Helena's Send pipeline routes a request through
  storage → session → chain → httpclient → scripting. These
  scenarios verify the happy path and the documented error paths
  without going through the UI.

  Scenario: GET request returns the response body
    Given a collection with a request "Hello" GET to the test server
    When I send "Hello"
    Then the response status is 200
    And the response body contains "hello world"

  Scenario: Unreachable URL surfaces the network error
    Given a collection with a request "Lost" GET to "http://127.0.0.1:1/"
    When I send "Lost"
    Then sending fails

  Scenario: Pre-script rewrites the URL before send
    Given the test server responds with 200 "OK" on "/right"
    And a collection with a request "Rewrite" GET to "{{base}}/wrong"
    And the request "Rewrite" has a pre-script that rewrites the URL to "{{base}}/right"
    When I send "Rewrite"
    Then the response status is 200

  Scenario: Post-script error is non-fatal
    Given a collection with a request "WithBadPost" GET to the test server
    And the request "WithBadPost" has a post-script that throws "boom"
    When I send "WithBadPost"
    Then the response status is 200
    And the last send recorded an error containing "boom"

  Scenario: Pre-script error stops Send before any HTTP fires
    Given a collection with a request "WithBadPre" GET to the test server
    And the request "WithBadPre" has a pre-script that throws "kaboom"
    When I send "WithBadPre"
    Then sending fails
    And the test server received no requests
