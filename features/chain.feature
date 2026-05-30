Feature: Chain — chain.Resolve end-to-end

  Helena's chain runner walks a request's Chain steps depth-first
  before the leaf, exposing each predecessor's response under
  chain.<alias> in the leaf's scripts. These scenarios cover the
  canonical chain shapes through every internal package without
  going through the UI.

  Scenario: Leaf reads a token from the chain step's JSON response
    Given a request "B" POST to "/login"
    And the test server responds with JSON on "/login":
      """
      {"token": "abc"}
      """
    And a request "A" GET to "/profile"
    And the request "A" chains "b" to "B"
    And the request "A" has a pre-script:
      """
      request.headers["Authorization"] = "Bearer " + chain.b.response.json.token;
      """
    When I send "A"
    Then the response status is 200
    And the test server received "/profile" with header "Authorization: Bearer abc"

  Scenario: Chain step inherits its folder's Bearer
    Given a folder "Auth" with Bearer "folder-token"
    And a request "Login" GET to "/login" inside folder "Auth"
    And a request "Profile" GET to "/profile"
    And the request "Profile" chains "login" to "Auth/Login"
    When I send "Profile"
    Then the response status is 200
    And the test server received "/login" with header "Authorization: Bearer folder-token"

  Scenario: Post-script env writes from a chain step reach the leaf
    Given a request "Bootstrap" POST to "/bootstrap"
    And the request "Bootstrap" has a post-script:
      """
      helena.env.set("TOKEN", "set-by-chain");
      """
    And a request "Leaf" GET to "/leaf?t={{TOKEN}}"
    And the request "Leaf" chains "boot" to "Bootstrap"
    When I send "Leaf"
    Then the response status is 200
    And the test server received "/leaf" with query "t=set-by-chain"

  Scenario: ID-pinned chain ref survives target rename
    Given a folder "Auth" with Bearer "x"
    And a request "Login" GET to "/login" inside folder "Auth"
    And a request "Profile" GET to "/profile"
    And the request "Profile" chains "login" to "Auth/Login" with pinned ID
    And I rename "Auth/Login" to "SignIn"
    When I send "Profile"
    Then the response status is 200
    And the test server received "/login"

  Scenario: Cycle through chain refs is detected
    Given a request "A" GET to "/a"
    And a request "B" GET to "/b"
    And the request "A" chains "b" to "B"
    And the request "B" chains "a" to "A"
    When I send "A"
    Then sending fails
    And the last send recorded an error containing "cycle"

  Scenario: Linear chain exceeding the depth cap surfaces a clear error
    Given a linear chain of depth 10
    When I send "R0"
    Then sending fails
    And the last send recorded an error containing "depth"
