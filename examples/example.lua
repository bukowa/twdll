if not package.loaded["libtwdll"] then
    local _lib = package.loadlib("libtwdll.dll", "luaopen_libtwdll")
    _lib()
end

-----@type CHARACTER_LIST_SCRIPT_INTERFACE
local chars = consul._game():model():world():faction_by_key("inv_rome"):character_list()

for i = 0, chars:num_items() - 1 do
    char = chars:item_at(i)
    if char:has_military_force() and char:military_force():unit_list():num_items() > 0 then
        twdll_unit.SetStrength(char:military_force():unit_list():item_at(0), 36550)
    end
end
