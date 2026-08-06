project = "twdll"
title = "twdll Lua API"
description = "Lua bindings for Total War: Attila"
full_description = [[Load the DLL from the campaign script — `campaigns/<campaign>/scripting.lua` — exactly as the test suite does.

twdll hooks game singletons (world, campaign UI, campaign model) on first load; those are only constructed once a campaign world exists. Calling singleton-dependent functions any earlier (e.g. from the main menu or a global script) can return `nil` or crash.

Not every function requires this: plain utilities (`twdll.core.Log`, `twdll.core.GameBuild`) and functions that read fixed module offsets work regardless of where they are called. This is how the current hook architecture works, not a permanent contract — it may change in the future.]]
not_luadoc = true
format = "markdown"
dir = "docs/lua/doc"
file = {}

