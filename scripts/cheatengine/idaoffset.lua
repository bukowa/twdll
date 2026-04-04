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
                    
                    local instText = lv.Selected.SubItems[0]
                    if not instText then return end
                    
                    local addrStr = instText:match("^(%x+)%s*%-")
                    
                    if addrStr then
                        local address = getAddress(addrStr)
                        local nameStr = getNameFromAddress(address)
                        local plusPos = string.find(nameStr, "+", 1, true)
                        
                        if plusPos then
                            -- Wycinamy sam offset
                            local offsetStr = string.sub(nameStr, plusPos + 1)
                            -- Zamieniamy offset na liczbę szesnastkową (base 16)
                            local offsetInt = tonumber(offsetStr, 16)
                            
                            if offsetInt then
                                -- DODAJEMY BAZĘ GHIDRY
                                local finalAddr = GHIDRA_BASE + offsetInt
                                
                                -- Formatujemy wynik z powrotem na HEX
                                local finalHex = string.format("%X", finalAddr)
                                writeToClipboard(finalHex)
                                print("Sukces: Skopiowano " .. finalHex .. " (Baza: " .. string.format("%X", GHIDRA_BASE) .. " + Offset: " .. offsetStr .. ")")
                            end
                        else
                            writeToClipboard(addrStr)
                            print("Skopiowano adres: " .. addrStr .. " (CE nie rozpoznał offsetu w module)")
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
