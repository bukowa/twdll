-- =========================================================
-- Target Ghidra / IDA Pro Image Base Address
-- Default in Ghidra for 64-bit: 0x140000000
-- Default for 32-bit game DLLs (e.g. empire.retail.dll): 0x10000000
local GHIDRA_BASE = 0x10000000 
-- =========================================================

registerFormAddNotification(function(f)
    if f.ClassName == "TFoundCodeDialog" then
        
        local tmr = createTimer(f)
        tmr.Interval = 100
        tmr.OnTimer = function(t)
            t.destroy()
            
            local lv = nil
            for i = 0, f.ComponentCount - 1 do
                if f.Component[i].ClassName == "TListView" then
                    lv = f.Component[i]
                    break
                end
            end
            
            if lv and lv.PopupMenu then
                local mi = createMenuItem(lv.PopupMenu)
                mi.Caption = "Copy Address for Ghidra/IDA"
                
                mi.OnClick = function()
                    if not lv.Selected then return end
                    
                    -- Extract text from list entry (format: Address - Bytes - Opcode)
                    local instText = lv.Selected.SubItems[0]
                    if not instText then return end
                    
                    -- Extract HEX address from the beginning of string
                    local addrStr = instText:match("^(%x+)%s*%-")
                    
                    if addrStr then
                        local address = tonumber(addrStr, 16)
                        if not address then address = getAddress(addrStr) end
                        
                        -- Find parent module (.exe / .dll) for this address
                        local modules = enumModules()
                        local rva = nil
                        local moduleName = ""
                        
                        for i, mod in ipairs(modules) do
                            -- Check if address falls within module memory bounds
                            if address >= mod.Address and address < (mod.Address + mod.Size) then
                                -- Calculate relative virtual address (RVA)
                                rva = address - mod.Address
                                moduleName = mod.Name
                                break
                            end
                        end
                        
                        if rva then
                            -- Add configured base to RVA
                            local finalAddr = GHIDRA_BASE + rva
                            local finalHex = string.format("%X", finalAddr)
                            
                            writeToClipboard(finalHex)
                            print(string.format("Copied %s (Module: %s | RVA: %X | Base: %X)", finalHex, moduleName, rva, GHIDRA_BASE))
                        else
                            -- Fallback if address is outside known modules
                            writeToClipboard(addrStr)
                            print("Copied absolute address: " .. addrStr .. " (Address not within any loaded module)")
                        end
                    else
                        print("Error: Unable to parse address from text: " .. instText)
                    end
                end
                
                lv.PopupMenu.Items.add(mi)
            end
        end
    end
end)
