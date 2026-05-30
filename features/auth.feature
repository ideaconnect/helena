Feature: Auth — application and inheritance

  Helena's auth layer applies the resolved Auth to outgoing requests:
  user-set Authorization always wins (Apply backs off), folders
  flatten via Inherit at Send time, and OAuth2 tokens cache so a
  second Send doesn't re-fetch from the IdP. These scenarios drive
  every supported auth type end-to-end.

  Scenario: Bearer auth lands as Authorization header
    Given a request "X" GET to "/x"
    And the request "X" has Bearer auth with "tok-123"
    When I send "X"
    Then the response status is 200
    And the test server received "/x" with header "Authorization: Bearer tok-123"

  Scenario: Basic auth lands as base64-encoded Authorization header
    Given a request "X" GET to "/x"
    And the request "X" has Basic auth with username "alice" password "s3cret"
    When I send "X"
    Then the response status is 200
    And the test server received "/x" with header "Authorization: Basic YWxpY2U6czNjcmV0"

  Scenario: API-Key in header lands as the named header
    Given a request "X" GET to "/x"
    And the request "X" has API-Key auth with name "X-Api-Key" value "key-xyz" in header
    When I send "X"
    Then the response status is 200
    And the test server received "/x" with header "X-Api-Key: key-xyz"

  Scenario: User-set Authorization header wins over Apply
    Given a request "X" GET to "/x"
    And the request "X" has Bearer auth with "should-not-apply"
    And the request "X" has a header "Authorization: manual-value"
    When I send "X"
    Then the response status is 200
    And the test server received "/x" with header "Authorization: manual-value"

  Scenario: Folder Bearer is inherited by AuthInherit request
    Given a folder "Auth" with Bearer "folder-token"
    And a request "Login" GET to "/login" inside folder "Auth"
    When I send "Auth/Login"
    Then the response status is 200
    And the test server received "/login" with header "Authorization: Bearer folder-token"

  Scenario: OAuth2 client_credentials fetches once and caches the token
    Given a request "X" GET to "/x"
    And the request "X" has OAuth2 client_credentials with token URL "{{base}}/oauth/token" client "ci" secret "cs"
    And the test server responds with JSON on "/oauth/token":
      """
      {"access_token":"auto-token","token_type":"Bearer","expires_in":3600}
      """
    When I send "X"
    Then the test server received "/x" with header "Authorization: Bearer auto-token"
    When I send "X"
    Then the test server received "/oauth/token" exactly 1 time
