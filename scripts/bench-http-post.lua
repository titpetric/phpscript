-- The POST /echo request for scripts/bench-http.sh.
--
-- wrk takes a method and a body only from Lua, and the route exists to price
-- form_value() parsing a body, so the body has to be a real urlencoded form.
wrk.method = "POST"
wrk.body = "name=sprint"
wrk.headers["Content-Type"] = "application/x-www-form-urlencoded"
