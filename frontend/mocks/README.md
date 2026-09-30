# mocks/

Test doubles for the API, used only by unit and component tests. Mocks are built from the
generated API types so they cannot drift from the contract. Nothing here is ever rendered by the
running application: the console shows real API data or an explicit empty/not-built state.
