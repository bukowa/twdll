-- =========================================================
-- TUTAJ WPISZ BAZĘ Z GHIDRY / IDA PRO (Z dopiskiem 0x na początku)
-- Domyślnie w Ghidrze dla 64-bit: 0x140000000
-- Często dla starszych DLL (32-bit): 0x10000000
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
                    
                    -- Pobranie tekstu z listy (zazwyczaj zawiera Adres - Bajty - Opcode)
                    local instText = lv.Selected.SubItems[0]
                    if not instText then return end
                    
                    -- Wyciągnięcie samego adresu HEX z początku stringa
                    local addrStr = instText:match("^(%x+)%s*%-")
                    
                    if addrStr then
                        local address = tonumber(addrStr, 16)
                        if not address then address = getAddress(addrStr) end
                        
                        -- Szukamy do jakiego modułu (.exe / .dll) należy ten adres
                        local modules = enumModules()
                        local rva = nil
                        local moduleName = ""
                        
                        for i, mod in ipairs(modules) do
                            -- Sprawdzamy czy adres mieści się w granicach modułu w pamięci
                            if address >= mod.Address and address < (mod.Address + mod.Size) then
                                -- Matematyczne wyliczenie czystego offsetu (RVA)
                                rva = address - mod.Address
                                moduleName = mod.Name
                                break
                            end
                        end
                        
                        if rva then
                            -- DODAJEMY BAZĘ GHIDRY DO RVA
                            local finalAddr = GHIDRA_BASE + rva
                            local finalHex = string.format("%X", finalAddr)
                            
                            writeToClipboard(finalHex)
                            print(string.format("Sukces: Skopiowano %s (Moduł: %s | RVA: %X | Baza: %X)", finalHex, moduleName, rva, GHIDRA_BASE))
                        else
                            -- Fallback, jeśli adres nie jest w żadnym .exe ani .dll (np. dynamicznie alokowana pamięć / JIT)
                            writeToClipboard(addrStr)
                            print("Skopiowano absolutny adres: " .. addrStr .. " (Adres nie należy do żadnego modułu w pamięci!)")
                        end
                    else
                        print("Błąd: Nie potrafię odczytać adresu z tekstu: " .. instText)
                    end
                end
                
                lv.PopupMenu.Items.add(mi)
            end
        end
    end
end)
