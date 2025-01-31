

-- a one-liner that collects all the keys and values of a table into a single string and returns it:
return (function(tbl) local str = "" for k, v in pairs(tbl) do str = str .. tostring(k) .. ": " .. tostring(v) .. "\n" end return str end)(require("lua_scripts.EpisodicScripting"))
