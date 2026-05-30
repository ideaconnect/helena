Feature: Persistence — Save and reload preserve state

  Helena's storage layer round-trips collections bit-for-bit: unknown
  YAML fields are kept via the Extras catch-all (invariant 1),
  Request.ID persists via info.id so chain refs survive reloads
  (Phase 7.4), and rename operations cascade to chain refs on disk.
  These scenarios drive the storage <-> session edge end-to-end.

  Scenario: Unknown YAML fields under a request survive Save and Load
    Given a hand-authored collection file at "opencollection.yml" with content:
      """
      info:
        name: ExtraColl
        type: collection
      settings:
        customKey: 42
      """
    And a hand-authored collection file at "scaffold.yml" with content:
      """
      info:
        name: Scaffold
        type: http
      http:
        method: GET
        url: https://x/
      scripts:
        tests:
          framework: external
      """
    And I open the collection
    When I save the active collection
    Then the file at "opencollection.yml" contains "settings:"
    And the file at "opencollection.yml" contains "customKey"
    And the file at "scaffold.yml" contains "tests:"
    And the file at "scaffold.yml" contains "framework: external"

  Scenario: Request.ID persists to info.id and survives reopen
    Given a request "Pinned" GET to "/pinned"
    And I save the active collection
    And I capture the ID of "Pinned"
    When I reopen the session
    Then "Pinned" still has the captured ID

  Scenario: Folder rename cascades to chain refs on disk
    Given a folder "Auth" with Bearer "x"
    And a request "Login" GET to "/login" inside folder "Auth"
    And a request "Profile" GET to "/profile"
    And the request "Profile" chains "login" to "Auth/Login"
    And I save the active collection
    When I rename "Auth" to "Authentication"
    And I save the active collection
    Then the file for "Profile" contains "Authentication/Login"

  Scenario: ChainStep.RequestID survives reopen
    Given a folder "Auth" with Bearer "x"
    And a request "Login" GET to "/login" inside folder "Auth"
    And a request "Profile" GET to "/profile"
    And the request "Profile" chains "login" to "Auth/Login" with pinned ID
    When I save the active collection
    And I reopen the session
    Then the file for "Profile" contains "requestId:"

  Scenario: Env overlay writes never persist across session reopens
    Given a request "X" GET to "/x"
    And the session env overlay sets "TOKEN" to "abc"
    When I save the active collection
    And I reopen the session
    Then the env overlay does not contain "TOKEN"
