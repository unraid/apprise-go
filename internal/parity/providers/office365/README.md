# office365 parity cases

`reply-to-multiple` was removed deliberately.

Upstream keeps reply-to addresses in a `set`, so the order they reach the
payload in is not defined. A fixture with two of them compares equal or not
depending on the Python build, which makes it a coin flip rather than a test.
`reply-to` covers the single-address case, where order cannot vary.

Multiple addresses are still covered, by
`TestOffice365ReplyToCarriesEveryAddress` in internal/notify, which asserts
every address is present without asserting their order.
