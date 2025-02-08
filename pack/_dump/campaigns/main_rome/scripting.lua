-------------------------------------------------------------------------------
------------------------------ SCRIPT SETUP -----------------------------------
-------------------------------------------------------------------------------
local core = require "ui/CoreUtils"
local advice = require "lua_scripts.export_advice"
local scripting = require "lua_scripts.EpisodicScripting"

-- Stephen
-- initialisation of lib_export_triggers
local triggers = require "data.lua_scripts.export_triggers"
out.ting("scripting.lua loaded");



local camera_pan = 0
local new_game = false
local panning = false
local player_faction = ""
local intro_advice_shown = false
local is_exiting_intro = false

EpisodicScripting = scripting
scripting.SetCampaign("main_rome")

local function OnNewCampaignStarted(context)
    new_game = true
    scripting.game_interface:set_zoom_limit(1.1, 0.7)
    scripting.game_interface:set_map_bounds(0, 700, 900, 0)
end

local function OnUICreated(context)

    CampaignUI.SetCameraMaxTiltAngle(1.15)
    CampaignUI.SetCameraMinDistance(8)

    -- Stephen
    -- initialise lib_export_triggers
    triggers.initialise_let(scripting);
end

local function ActivateCinematicCam(l)
    if (l == true) then
        out.ting("Activating cinematic cam")
        scripting.game_interface:override_ui("disable_event_panel_auto_open", true);
        CampaignUI.ToggleCinematicBorders(true)
        scripting.game_interface:take_shroud_snapshot()
        scripting.game_interface:make_neighbouring_regions_visible_in_shroud()
        -- scripting.game_interface:override_ui("disable_settlement_labels", true)
        scripting.game_interface:override_ui("disable_advice_changes", true)
        scripting.game_interface:steal_user_input(true)
        new_game = false
    else
        out.ting("De-activating cinematic cam")
        scripting.game_interface:override_ui("disable_event_panel_auto_open", false);
        scripting.game_interface:restore_shroud_from_snapshot()
        -- scripting.game_interface:override_ui("disable_settlement_labels", false)
        scripting.game_interface:override_ui("disable_advice_changes", false)
        scripting.game_interface:steal_user_input(false)
        CampaignUI.ToggleCinematicBorders(false)
    end
end

-------------------------------------------------------------------------------------------------------------------------
--
--
-- FACTION INTROS AND CAMERA PANS
--

-- TRIGGER A PAN BASED UPON A PIECE OF ADVICE TRIGGERING

local function OnAdviceIssued(context)
    panning = true
    -- ROME
    if conditions.AdviceJustDisplayed("2139660094", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(10,
                {209.0, 289.0, 0.9, 0.0},
                {200.0, 312, 0.9, 0.235})
        camera_pan = 100
        -- CARTHAGE
    elseif conditions.AdviceJustDisplayed("2139660101", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(2,
                {186.0, 211.0, 0.9, 0.0},
                {186.0, 211.0, 0.9, -0.35})
        camera_pan = 110
        -- EGYPT
    elseif conditions.AdviceJustDisplayed("2139660107", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(3,
                {355.0, 155.0, 0.9, 0.15},
                {355.0, 155.0, 0.9, -0.15})
        camera_pan = 120
        -- MACEDON
    elseif conditions.AdviceJustDisplayed("2139660108", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(3,
                {301.0, 279.0, 0.9, 0.0},
                {301.0, 279.0, 0.9, 0.26})
        camera_pan = 130
        -- BRITON
    elseif conditions.AdviceJustDisplayed("2139660109", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(6,
                {112.0, 436.0, 0.9, 0.0},
                {112.0, 434.0, 0.9001, -0.55})
        camera_pan = 140
        -- GAUL
    elseif conditions.AdviceJustDisplayed("2139660111", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(7,
                {128.0, 344.0, 1.0, 0.0},
                {142.0, 363.0, 1.0, 0.47})
        camera_pan = 150
        -- GERMANIC
    elseif conditions.AdviceJustDisplayed("2139660112", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(7,
                {228.0, 429.0, 0.9, 0.8},
                {234.0, 410.0, 0.9, 0.5})
        camera_pan = 160
        -- PARTHIA
    elseif conditions.AdviceJustDisplayed("2139660116", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(6,
                {588.0, 248.0, 0.9, 0.0},
                {588.0, 248.0, 0.9, -0.75})
        camera_pan = 170
        -- PONTUS
    elseif conditions.AdviceJustDisplayed("2139660123", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(6,
                {425.0, 279.0, 0.9, 0.15},
                {425.0, 279.0, 0.9, -0.15})
        camera_pan = 180
        -- ATHENS
    elseif conditions.AdviceJustDisplayed("2139660126", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(2,
                {305.0, 248.0, 0.9, -0.1},
                {305.0, 248.0, 0.9, 0.1})
        camera_pan = 190
        -- SPARTA
    elseif conditions.AdviceJustDisplayed("2139660132", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(3,
                {291.0, 238.0, 0.9, -0.2},
                {291.0, 238.0, 0.9, 0.0})
        camera_pan = 200
        -- EPIRUS
    elseif conditions.AdviceJustDisplayed("2139660135", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(8,
                {266.0, 275.0, 0.9, 0.0},
                {266.0, 275.0, 0.9, 0.4})
        camera_pan = 210
        -- SELEUCID
    elseif conditions.AdviceJustDisplayed("2139663554", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(5,
                {415.0, 231.0, 0.9, 0.0},
                {415.0, 231.0, 0.9, 0.25})
        camera_pan = 220
        -- SCYTHIA
    elseif conditions.AdviceJustDisplayed("2139663562", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(3,
                {403.0, 364.0, 0.9, 0.0},
                {403.0, 364.0, 0.9, -0.25})
        camera_pan = 230
        -- ROXOLANI
    elseif conditions.AdviceJustDisplayed("2139663567", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(5,
                {508.0, 426.0, 0.9, 0.0},
                {508.0, 426.0, 0.9, 0.3})
        camera_pan = 240
        -- MASSAGETAE
    elseif conditions.AdviceJustDisplayed("2139663566", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(4,
                {552.0, 319.0, 0.9, 0.0},
                {552.0, 319.0, 0.9, 0.15})
        camera_pan = 250
        -- BAKTRIA
    elseif conditions.AdviceJustDisplayed("2139663577", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(4,
                {645.0, 239.0, 0.75, 0.0},
                {645.0, 239.0, 0.75, -0.3})
        camera_pan = 260
        -- NERVII
    elseif conditions.AdviceJustDisplayed("2139663579", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(5,
                {156.0, 426.0, 0.9, 0.0},
                {156.0, 426.0, 0.9, 0.2})
        camera_pan = 270
        -- BOII
    elseif conditions.AdviceJustDisplayed("2139663585", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(3,
                {260.0, 389.0, 0.9, 0.0},
                {260.0, 389.0, 0.75, -0.2})
        camera_pan = 280
        -- GALATIA
    elseif conditions.AdviceJustDisplayed("2139663576", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(6,
                {390.0, 271.0, 0.85, -0.2},
                {390.0, 271.0, 0.85, 0.2})
        camera_pan = 290
        -- LUSITANI
    elseif conditions.AdviceJustDisplayed("2139663758", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(12,
                {18.3, 250.7, 1.05, 0},
                {18.3, 250.7, 0.76, 0})
        camera_pan = 300
        -- AREVACI
    elseif conditions.AdviceJustDisplayed("2139663766", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(8,
                {77.0, 278.1, 1.0, 0},
                {77.0, 278.1, 0.76, 0})
        camera_pan = 310
        -- SYRACUSE
    elseif conditions.AdviceJustDisplayed("2139663751", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(6,
                {224.7, 221.0, 1.05, 0},
                {224.7, 221.0, 0.76, 0})
        camera_pan = 320
        -- TYLIS
    elseif conditions.AdviceJustDisplayed("2139663935", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(8,
                {345.63, 296.62, 1.05, 0},
                {345.63, 296.62, 0.76, 0})
        camera_pan = 330
        -- ODRYSIAN KINGDOM
    elseif conditions.AdviceJustDisplayed("2139663945", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(5,
                {345.03, 315.03, 1.05, 0},
                {345.03, 315.03, 0.76, 0})
        camera_pan = 340
        -- GETAE
    elseif conditions.AdviceJustDisplayed("2139663948", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(6,
                {316.2, 328.4, 1.05, 0},
                {316.2, 328.4, 0.76, 0})
        camera_pan = 350
        -- ARDIAEI
    elseif conditions.AdviceJustDisplayed("2139663950", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(7,
                {270.37, 296.63, 1.05, 0},
                {270.37, 296.63, 0.76, 0})
        camera_pan = 360
        -- ARMENIA
    elseif conditions.AdviceJustDisplayed("2139663987", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(8,
                {482.01, 279.89, 1.05, 0},
                {482.01, 279.89, 0.76, 0})
        camera_pan = 370
        -- MASSILIA
    elseif conditions.AdviceJustDisplayed("2139663973", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(5.5,
                {150.406403, 306.824707, 1.05, 0},
                {150.406403, 306.824707, 0.76, 0})
        camera_pan = 380
        -- PERGAMON
    elseif conditions.AdviceJustDisplayed("2139663977", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(6,
                {342.926605, 265.142883, 1.05, 0},
                {342.926605, 265.142883, 0.76, 0})
        camera_pan = 390
        -- CIMMERIA
    elseif conditions.AdviceJustDisplayed("2139663976", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(4,
                {413.116241, 340.401733, 1.05, 0},
                {413.116241, 340.401733, 0.76, 0})
        camera_pan = 400
        -- COLCHIS
    elseif conditions.AdviceJustDisplayed("2139663979", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(6,
                {461.246277, 309.526306, 1.05, 0},
                {461.246277, 309.526306, 0.76, 0})
        camera_pan = 410
        -- KUSH
    elseif conditions.AdviceJustDisplayed("2139664741", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(6,
                {400.877167, 42.273277, 1.05, 0},
                {400.877167, 42.273277, 0.76, 0})
        camera_pan = 420
        -- NABATEA
    elseif conditions.AdviceJustDisplayed("2139664754", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(2,
                {416.526184, 154.017838, 1.05, 0},
                {416.526184, 154.017838, 0.76, 0})
        camera_pan = 430
        -- SABA
    elseif conditions.AdviceJustDisplayed("2139664745", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(3,
                {492.328552, 73.835579, 1.05, 0},
                {492.328552, 73.835579, 0.76, 0})
        camera_pan = 440
        -- MASAESYLI
    elseif conditions.AdviceJustDisplayed("2139664742", context) and not CampaignUI.IsMultiplayer() and intro_advice_shown == false then
        out.ting("intro advice issued")
        ActivateCinematicCam(true)
        intro_advice_shown = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {130.716553, 184.278084, 1.05, 0},
                {130.716553, 184.278084, 0.76, 0})
        camera_pan = 450
    end
end

-- TRIGGER A CAMERA PAN BASED UPON A PREVIOUS CAMERA PAN ENDING

local function OnCameraMoverFinished(context)
    if camera_pan ~= 0 then
        out.ting("DEBUG: OnCameraMoverFinished:"..tostring(camera_pan))
    end
    -- ROME
    if camera_pan == 100 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {200.0, 312, 0.9, 0.235},
                {216.0, 318.0, 0.9, 0.75})
        camera_pan = 101
    elseif camera_pan == 101 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {216.0, 318.0, 0.9, 0.75},
                {199.0, 235.0, 0.72, 2.2})
        camera_pan = 102
    elseif camera_pan == 102 then
        scripting.game_interface:scroll_camera_with_direction(8,
                {199.0, 235.0, 0.72, 2.2},
                {225.0, 222.0, 0.72, 0.2})
        camera_pan = 103
    elseif camera_pan == 103 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {225.0, 222.0, 0.72, 0.2},
                {209.0, 289.0, 0.9, 0.0})
        camera_pan = 108
    elseif camera_pan == 108 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {209.0, 289.0, 0.9, 0.0},
                {209.0, 289.0, 0.9001, 0.0})
        camera_pan = 109
    elseif camera_pan == 109 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(209.0, 289.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_rome", 0.01)


        --CARTHAGE
    elseif camera_pan == 110 then
        scripting.game_interface:scroll_camera_with_direction(12,
                {186.0, 211.0, 0.9, -0.35},
                {200.0, 234.0, 0.9, -1.05},
                {175.0, 249.0, 0.9, -1.49},
                {95.0, 223.0, 0.9, -1.20},
                {57.0, 235.0, 0.9, -0.33})
        camera_pan = 111
    elseif camera_pan == 111 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {57.0, 235.0, 0.9, -0.33},
                {57.0, 235.0, 0.9, 0.82})
        camera_pan = 112
    elseif camera_pan == 112 then
        scripting.game_interface:scroll_camera_with_direction(7,
                {57.0, 235.0, 0.9, 0.82},
                {159.0, 243.0, 0.9, 0.87},
                {176.0, 284.0, 0.9, 0.29})
        camera_pan = 113
    elseif camera_pan == 113 then
        scripting.game_interface:scroll_camera_with_direction(7,
                {176.0, 284.0, 0.9, 0.29},
                {203.0, 291.0, 0.78, 1.28},
                {230.0, 253.0, 0.78, 0.78})
        camera_pan = 114
    elseif camera_pan == 114 then
        scripting.game_interface:scroll_camera_with_direction(7,
                {230.0, 253.0, 0.78, 0.78},
                {183.0, 208.0, 0.78, 0.44},
                {192.0, 190.0, 0.78, 0.35},
                {249.0, 156.0, 0.78, -0.2})
        camera_pan = 115
    elseif camera_pan == 115 then
        scripting.game_interface:scroll_camera_with_direction(1,
                {249.0, 156.0, 0.78, -0.2},
                {249.0, 156.0, 0.78, -0.25})
        camera_pan = 116
    elseif camera_pan == 116 then
        scripting.game_interface:scroll_camera_with_direction(2,
                {249.0, 156.0, 0.78, -0.25},
                {186.0, 211.0, 0.9, 0.0})
        camera_pan = 118
    elseif camera_pan == 118 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {186.0, 211.0, 0.9, 0.0},
                {186.0, 211.0, 0.9001, 0.0})
        camera_pan = 119
    elseif camera_pan == 119 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(186.0, 211.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_carthage", 0.01)


        --EGYPT
    elseif camera_pan == 120 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {355.0, 155.0, 0.9, -0.15},
                {331.0, 161.0, 1.0, 0.0})
        camera_pan = 121
    elseif camera_pan == 121 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {331.0, 161.0, 1.0, 0.0},
                {331.0, 161.0, 1.0, 0.25})
        camera_pan = 122
    elseif camera_pan == 122 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {331.0, 161.0, 1.0, 0.25},
                {398.0, 215.0, 0.9, 0.3})
        camera_pan = 123
    elseif camera_pan == 123 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {398.0, 215.0, 0.9, 0.3},
                {407.0, 231.0, 0.8, 0.85})
        camera_pan = 124
    elseif camera_pan == 124 then
        scripting.game_interface:scroll_camera_with_direction(9,
                {407.0, 231.0, 0.8, 0.85},
                {412.0, 232.0, 0.8, 1.75},
                {408.0, 200.0, 0.8, 2.45})
        camera_pan = 125
    elseif camera_pan == 125 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {408.0, 200.0, 0.8, 2.45},
                {424.0, 113.0, 0.9, 1.5})
        camera_pan = 126
    elseif camera_pan == 126 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {424.0, 113.0, 0.9, 1.51},
                {424.0, 113.0, 0.9, 0.0})
        camera_pan = 127
    elseif camera_pan == 127 then
        scripting.game_interface:scroll_camera_with_direction(8,
                {424.0, 113.0, 0.9, 0.0},
                {357.0, 156.0, 0.9, 0.0})
        camera_pan = 128
    elseif camera_pan == 128 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {357.0, 156.0, 0.9, 0.0},
                {357.0, 156.0, 0.9001, 0.0})

        camera_pan = 129
    elseif camera_pan == 129 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(358.0, 155.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_egypt", 0.01)


        --MACEDON
    elseif camera_pan == 130 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {301.0, 279.0, 0.9, 0.26},
                {345.0, 296.0, 0.9, 0.26})
        camera_pan = 131
    elseif camera_pan == 131 then
        scripting.game_interface:scroll_camera_with_direction(2,
                {345.0, 296.0, 0.9, 0.26},
                {345.0, 296.0, 0.9, -0.3})
        camera_pan = 132
    elseif camera_pan == 132 then
        scripting.game_interface:scroll_camera_with_direction(10,
                {345.0, 296.0, 0.9, -0.3},
                {306.0, 253.0, 0.9, -1.9},
                {291.0, 237.0, 0.9, -0.63})
        camera_pan = 133
    elseif camera_pan == 133 then
        scripting.game_interface:scroll_camera_with_direction(8,
                {291.0, 237.0, 0.9, -0.63},
                {293.0, 263.0, 0.9, 0.1},
                {268.0, 274.0, 0.9, -0.1})
        camera_pan = 134
    elseif camera_pan == 134 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {268.0, 274.0, 0.9, -0.1},
                {268.0, 274.0, 0.9, 0.5})
        camera_pan = 135
    elseif camera_pan == 135 then
        scripting.game_interface:scroll_camera_with_direction(10,
                {268.0, 274.0, 0.9, 0.5},
                {301.0, 279.0, 0.9, 0.0})
        camera_pan = 138
    elseif camera_pan == 138 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {301.0, 279.0, 0.9, 0.0},
                {301.0, 279.0, 0.9001, 0.0})
        camera_pan = 139
    elseif camera_pan == 139 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(301.0, 279.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_macedon", 0.01)


        --BRITON
    elseif camera_pan == 140 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {112.0, 434.0, 0.9001, -0.55},
                {70.0, 434.0, 0.9, -0.85})
        camera_pan = 141
    elseif camera_pan == 141 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {70.0, 434.0, 0.9, -0.85},
                {70.0, 434.0, 0.9, 0.25})
        camera_pan = 142
    elseif camera_pan == 142 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {70.0, 434.0, 0.9, 0.25},
                {90.0, 466.0, 0.9, 0.95})
        camera_pan = 143
    elseif camera_pan == 143 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {90.0, 466.0, 0.9, 0.95},
                {90.0, 466.0, 0.9, 1.4})
        camera_pan = 144
    elseif camera_pan == 144 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {90.0, 466.0, 0.9, 1.4},
                {132.0, 417.0, 0.9, 1.8})
        camera_pan = 145
    elseif camera_pan == 145 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {132.0, 417.0, 0.9, 1.8},
                {132.0, 417.0, 0.9, -0.6})
        camera_pan = 146
    elseif camera_pan == 146 then
        scripting.game_interface:scroll_camera_with_direction(8,
                {132.0, 417.0, 0.9, -0.6},
                {86.0, 419.0, 0.9, -0.9},
                {77.0, 420.0, 0.9, 0.45})
        camera_pan = 147
    elseif camera_pan == 147 then
        scripting.game_interface:scroll_camera_with_direction(8,
                {77.0, 420.0, 0.9, 0.45},
                {112.0, 436.0, 0.9, 0.0})
        camera_pan = 148
    elseif camera_pan == 148 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {112.0, 436.0, 0.9, 0.0},
                {112.0, 436.0, 0.9001, 0.0})
        camera_pan = 149
    elseif camera_pan == 149 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(112.0, 436.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_briton", 0.01)


        --GAUL
    elseif camera_pan == 150 then
        scripting.game_interface:scroll_camera_with_direction(7,
                {142.0, 363.0, 1.0, 0.47},
                {142.0, 363.0, 1.0, -1.0})
        camera_pan = 151
    elseif camera_pan == 151 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {142.0, 363.0, 1.0, -1.0},
                {106.0, 355.0, 1.0, -1.6})
        camera_pan = 152
    elseif camera_pan == 152 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {106.0, 355.0, 1.0, -1.6},
                {106.0, 355.0, 1.0, 0.5})
        camera_pan = 153
    elseif camera_pan == 153 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {106.0, 355.0, 1.0, 0.5},
                {99.0, 332.0, 1.0, 1.5})
        camera_pan = 154
    elseif camera_pan == 154 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {99.0, 332.0, 1.0, 1.5},
                {99.0, 332.0, 1.0, 2.0})
        camera_pan = 155
    elseif camera_pan == 155 then
        scripting.game_interface:scroll_camera_with_direction(7,
                {99.0, 332.0, 1.0, 2.0},
                {149.0, 305.0, 1.0, 1.2})

        camera_pan = 156
    elseif camera_pan == 156 then
        scripting.game_interface:scroll_camera_with_direction(8,
                {149.0, 305.0, 1.0, 1.2},
                {149.0, 305.0, 1.0, 0.0})

        camera_pan = 157
    elseif camera_pan == 157 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {149.0, 305.0, 1.0, 0.},
                {128.0, 344.0, 1.0, 0.0})
        camera_pan = 158
    elseif camera_pan == 158 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {128.0, 344.0, 1.0, 0.0},
                {128.0, 344.0, 1.001, 0.0})
        camera_pan = 159
    elseif camera_pan == 159 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(1.0)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(128.0, 344.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_gaul", 0.1)


        --GERMANIC
    elseif camera_pan == 160 then
        scripting.game_interface:scroll_camera_with_direction(9,
                {234.0, 410.0, 0.9, 0.5},
                {234.0, 410.0, 0.9, -0.8})
        camera_pan = 161
    elseif camera_pan == 161 then
        scripting.game_interface:scroll_camera_with_direction(29,
                {234.0, 410.0, 0.9, -0.8},
                {203.0, 399.0, 0.8, -1.1},
                {185.0, 405.0, 0.8, 0.35},
                {187.0, 438.0, 0.8, 0.7},
                {218.0, 469.0, 0.8, 1.21},
                {256.0, 457.0, 0.8, 3.04},
                {228.0, 429.0, 0.9, 0.8})
        camera_pan = 162
    elseif camera_pan == 162 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {228.0, 429.0, 0.9, 0.8},
                {228.0, 429.0, 0.9, 0.0})
        camera_pan = 168
    elseif camera_pan == 168 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {228.0, 429.0, 0.9, 0.0},
                {228.0, 429.0, 0.9001, 0.0})
        camera_pan = 169
    elseif camera_pan == 169 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(228.0, 429.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_germanic", 0.01)


        --PARTHIA
    elseif camera_pan == 170 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {588.0, 248.0, 0.9, -0.75},
                {558.0, 217.0, 0.9, -1.35})
        camera_pan = 171
    elseif camera_pan == 171 then
        scripting.game_interface:scroll_camera_with_direction(11,
                {558.0, 217.0, 0.9, -1.35},
                {558.0, 217.0, 0.9, 0.35})
        camera_pan = 172
    elseif camera_pan == 172 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {558.0, 217.0, 0.9, 0.35},
                {598.0, 301.0, 0.9, 0.92})
        camera_pan = 173
    elseif camera_pan == 173 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {598.0, 301.0, 0.9, 0.92},
                {598.0, 301.0, 0.9, 1.2})
        camera_pan = 174
    elseif camera_pan == 174 then
        scripting.game_interface:scroll_camera_with_direction(7,
                {598.0, 301.0, 0.9, 1.2},
                {604.0, 247.0, 0.72, 1.65},
                {613.0, 207.0, 0.72, 1.81})
        camera_pan = 175
    elseif camera_pan == 175 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {613.0, 207.0, 0.72, 1.81},
                {613.0, 207.0, 0.72, 0.0})
        camera_pan = 176
    elseif camera_pan == 176 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {613.0, 207.0, 0.72, 0.0},
                {588.0, 248.0, 0.9, 0.0})
        camera_pan = 178
    elseif camera_pan == 178 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {588.0, 248.0, 0.9, 0.0},
                {588.0, 248.0, 0.9001, 0.0})
        camera_pan = 179
    elseif camera_pan == 179 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(588.0, 248.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_parthia", 0.01)


        --PONTUS
    elseif camera_pan == 180 then
        scripting.game_interface:scroll_camera_with_direction(13,
                {425.0, 279.0, 0.9, -0.15},
                {428.0, 256.0, 0.9, -1.0},
                {406.0, 257.0, 0.9, -0.75},
                {391.0, 272.0, 0.9, -0.6})
        camera_pan = 181
    elseif camera_pan == 181 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {391.0, 272.0, 0.9, -0.6},
                {391.0, 272.0, 0.9, -0.9})
        camera_pan = 182
    elseif camera_pan == 182 then
        scripting.game_interface:scroll_camera_with_direction(7,
                {391.0, 272.0, 0.9, -0.9},
                {368.0, 284.0, 0.9, -0.95},
                {305.0, 248.0, 0.9, -1.4})
        camera_pan = 183
    elseif camera_pan == 183 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {305.0, 248.0, 0.9, -1.4},
                {305.0, 248.0, 0.9, 0.0})
        camera_pan = 184
    elseif camera_pan == 184 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {305.0, 248.0, 0.9, 0.0},
                {361.0, 281.0, 0.9, 1.3},
                {460.0, 308.0, 0.9, 1.3})
        camera_pan = 185
    elseif camera_pan == 185 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {460.0, 308.0, 0.9, 1.3},
                {460.0, 308.0, 0.9, 0.0})
        camera_pan = 186
    elseif camera_pan == 186 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {460.0, 308.0, 0.9, 0.0},
                {425.0, 279.0, 0.9, 0.0})
        camera_pan = 188
    elseif camera_pan == 188 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {425.0, 279.0, 0.9, 0.0},
                {425.0, 279.0, 0.9001, 0.0})
        camera_pan = 189
    elseif camera_pan == 189 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(425.0, 279.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_pontus", 0.01)


        --ATHENS
    elseif camera_pan == 190 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {305.0, 248.0, 0.9, 0.1},
                {302.0, 275.0, 0.9, 0.1})
        camera_pan = 191
    elseif camera_pan == 191 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {302.0, 275.0, 0.9, 0.1},
                {307.0, 283.0, 0.9, -1.5})
        camera_pan = 192
    elseif camera_pan == 192 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {307.0, 283.0, 0.9, -1.5},
                {293.0, 242.0, 0.9, -2.5})
        camera_pan = 193
    elseif camera_pan == 193 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {293.0, 242.0, 0.9, -2.5},
                {294.0, 236.0, 0.9, -0.7})
        camera_pan = 194
    elseif camera_pan == 194 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {294.0, 236.0, 0.9, -0.7},
                {294.0, 264.0, 0.9, -0.9})

        camera_pan = 195
    elseif camera_pan == 195 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {294.0, 264.0, 0.9, -0.9},
                {294.0, 266.0, 0.9, -1.7})
        camera_pan = 196
    elseif camera_pan == 196 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {294.0, 266.0, 0.9, -1.7},
                {305.0, 248.0, 0.9, -0.7})
        camera_pan = 197
    elseif camera_pan == 197 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {305.0, 248.0, 0.9, -0.7},
                {305.0, 248.0, 0.9, 0.0})
        camera_pan = 198
    elseif camera_pan == 198 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {305.0, 248.0, 0.9, 0.0},
                {305.0, 248.0, 0.9001, 0.0})
        camera_pan = 199
    elseif camera_pan == 199 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(305.0, 248.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_athens", 0.01)


        --SPARTA
    elseif camera_pan == 200 then
        scripting.game_interface:scroll_camera_with_direction(10,
                {291.0, 238.0, 0.9, 0.0},
                {272.0, 259.0, 0.9, -0.05},
                {264.0, 272.0, 0.9, 0.7},
                {280.0, 265.0, 0.9, 1.1},
                {292.0, 260.0, 0.9, 0.0})
        camera_pan = 201
    elseif camera_pan == 201 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {292.0, 260.0, 0.9, 0.0},
                {292.0, 260.0, 0.9, -0.25})
        camera_pan = 202
    elseif camera_pan == 202 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {292.0, 260.0, 0.9, -0.25},
                {301.0, 279.0, 0.9, -0.25})
        camera_pan = 203
    elseif camera_pan == 203 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {301.0, 279.0, 0.9, -0.25},
                {301.0, 279.0, 0.9, -0.8})
        camera_pan = 204
    elseif camera_pan == 204 then
        scripting.game_interface:scroll_camera_with_direction(7,
                {301.0, 279.0, 0.9, -0.8},
                {308.0, 250.0, 0.9, -1.5})
        camera_pan = 205
    elseif camera_pan == 205 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {308.0, 250.0, 0.9, -1.5},
                {308.0, 250.0, 0.9, -0.95})
        camera_pan = 206
    elseif camera_pan == 206 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {308.0, 250.0, 0.9, -0.95},
                {291.0, 238.0, 0.9, 0.0})
        camera_pan = 208
    elseif camera_pan == 208 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {291.0, 238.0, 0.9, 0.0},
                {291.0, 238.0, 0.9001, 0.0})
        camera_pan = 209
    elseif camera_pan == 209 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(291.0, 238.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_sparta", 0.01)


        --EPIRUS
    elseif camera_pan == 210 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {266.0, 275.0, 0.9, 0.4},
                {289.0, 236.0, 0.9, 0.4})
        camera_pan = 211
    elseif camera_pan == 211 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {289.0, 236.0, 0.9, 0.4},
                {289.0, 236.0, 0.9, 0.1})
        camera_pan = 212
    elseif camera_pan == 212 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {289.0, 236.0, 0.9, 0.1},
                {306.0, 246.0, 0.9, 0.0})
        camera_pan = 213
    elseif camera_pan == 213 then
        scripting.game_interface:scroll_camera_with_direction(7,
                {306.0, 246.0, 0.9, 0.0},
                {307.0, 247.0, 0.9, -0.3})
        camera_pan = 214
    elseif camera_pan == 214 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {307.0, 247.0, 0.9, -0.3},
                {302.0, 279.0, 0.9, -0.3})
        camera_pan = 215
    elseif camera_pan == 215 then
        scripting.game_interface:scroll_camera_with_direction(8,
                {302.0, 279.0, 0.9, -0.3},
                {303.0, 280.0, 0.9, -0.75})
        camera_pan = 216
    elseif camera_pan == 216 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {303.0, 280.0, 0.9, -0.75},
                {249.0, 273.0, 0.9, -1.4})
        camera_pan = 217
    elseif camera_pan == 217 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {249.0, 273.0, 0.9, -1.4},
                {249.0, 273.0, 0.9, 0.0})
        camera_pan = 2172
    elseif camera_pan == 2172 then
        scripting.game_interface:scroll_camera_with_direction(2,
                {249.0, 273.0, 0.9, 0.0},
                {266.0, 275.0, 0.9, 0.0})
        camera_pan = 218
    elseif camera_pan == 218 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {266.0, 275.0, 0.9, 0.0},
                {266.0, 275.0, 0.9001, 0.0})
        camera_pan = 219
    elseif camera_pan == 219 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(266.0, 275.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_epirus", 0.01)


        --SELEUCID
    elseif camera_pan == 220 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {415.0, 231.0, 0.9, 0.25},
                {486.0, 190.0, 0.7, 0.7},
                {557.0, 215.0, 0.7, 0.0})
        camera_pan = 221
    elseif camera_pan == 221 then
        scripting.game_interface:scroll_camera_with_direction(2,
                {557.0, 215.0, 0.7, 0.0},
                {557.0, 215.0, 0.7, -0.15})
        camera_pan = 222
    elseif camera_pan == 222 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {557.0, 215.0, 0.7, -0.15},
                {405.0, 169.0, 0.7, -0.4})
        camera_pan = 223
    elseif camera_pan == 223 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {405.0, 169.0, 0.7, -0.4},
                {405.0, 169.0, 0.7, 0.0})
        camera_pan = 224
    elseif camera_pan == 224 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {405.0, 169.0, 0.7, 0.0},
                {388.0, 249.0, 0.7, 0.0})
        camera_pan = 225
    elseif camera_pan == 225 then
        scripting.game_interface:scroll_camera_with_direction(10,
                {388.0, 249.0, 0.7, 0.0},
                {391.0, 271.0, 0.9, 0.0},
                {364.0, 281.0, 0.9, -0.4},
                {343.0, 263.0, 0.9, -0.4})
        camera_pan = 226
    elseif camera_pan == 226 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {343.0, 263.0, 0.9, -0.4},
                {415.0, 231.0, 0.9002, 0.0})
        camera_pan = 228
    elseif camera_pan == 228 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {415.0, 231.0, 0.9002, 0.0},
                {415.0, 231.0, 0.9, 0.0})
        camera_pan = 229
    elseif camera_pan == 229 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(415.0, 231.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_seleucid", 0.01)


        --SCYTHIA
    elseif camera_pan == 230 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {403.0, 364.0, 0.9, -0.25},
                {373.0, 361.0, 0.9, -0.25})
        camera_pan = 231
    elseif camera_pan == 231 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {373.0, 361.0, 0.9, -0.25},
                {373.0, 361.0, 0.9, 0.2})
        camera_pan = 232
    elseif camera_pan == 232 then
        scripting.game_interface:scroll_camera_with_direction(12,
                {373.0, 361.0, 0.9, 0.2},
                {400.0, 355.0, 0.9, 0.3},
                {468.0, 342.0, 0.9, 0.35},
                {504.0, 322.0, 0.9, 0.4})
        camera_pan = 233
    elseif camera_pan == 233 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {504.0, 322.0, 0.9, 0.4},
                {504.0, 322.0, 0.9, -0.1})
        camera_pan = 234
    elseif camera_pan == 234 then
        scripting.game_interface:scroll_camera_with_direction(7,
                {504.0, 322.0, 0.9, -0.1},
                {413.0, 341.0, 0.9, -0.3})
        camera_pan = 235
    elseif camera_pan == 235 then
        scripting.game_interface:scroll_camera_with_direction(8,
                {413.0, 341.0, 0.9, -0.3},
                {413.0, 341.0, 0.9, 0.0})
        camera_pan = 236
    elseif camera_pan == 236 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {413.0, 341.0, 0.9, 0.0},
                {403.0, 364.0, 0.9002, 0.0})
        camera_pan = 238
    elseif camera_pan == 238 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {403.0, 364.0, 0.9002, 0.0},
                {403.0, 364.0, 0.9, 0.0})
        camera_pan = 239
    elseif camera_pan == 239 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(403.0, 364.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_scythia", 0.01)


        --ROXOLANI
    elseif camera_pan == 240 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {508.0, 426.0, 0.9, 0.3},
                {547.0, 372.0, 0.9, 0.4})
        camera_pan = 241
    elseif camera_pan == 241 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {547.0, 372.0, 0.9, 0.4},
                {547.0, 372.0, 0.9, 0.0})
        camera_pan = 242
    elseif camera_pan == 242 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {547.0, 372.0, 0.9, 0.0},
                {505.0, 321.0, 0.7, -0.45})
        camera_pan = 243
    elseif camera_pan == 243 then
        scripting.game_interface:scroll_camera_with_direction(9,
                {505.0, 321.0, 0.7, -0.45},
                {505.0, 321.0, 0.7, 0.0})
        camera_pan = 244
    elseif camera_pan == 244 then
        scripting.game_interface:scroll_camera_with_direction(8,
                {505.0, 321.0, 0.7, 0.0},
                {508.0, 426.0, 0.9, 0.0})
        camera_pan = 248
    elseif camera_pan == 248 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {508.0, 426.0, 0.9002, 0.0},
                {508.0, 426.0, 0.9, 0.0})
        camera_pan = 249
    elseif camera_pan == 249 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(508.0, 426.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_roxolani", 0.01)


        --MASSAGETAE
    elseif camera_pan == 250 then
        scripting.game_interface:scroll_camera_with_direction(7,
                {552.0, 319.0, 0.9, 0.15},
                {547.0, 373.0, 0.9, 0.25})
        camera_pan = 251
    elseif camera_pan == 251 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {547.0, 373.0, 0.9, 0.25},
                {547.0, 373.0, 0.9, 0.35})
        camera_pan = 252
    elseif camera_pan == 252 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {547.0, 373.0, 0.9, 0.35},
                {559.0, 338.0, 0.9, 0.65},
                {599.0, 300.0, 0.9, 0.85})
        camera_pan = 253
    elseif camera_pan == 253 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {599.0, 300.0, 0.9, 0.85},
                {599.0, 300.0, 0.9, 0.6})
        camera_pan = 254
    elseif camera_pan == 254 then
        scripting.game_interface:scroll_camera_with_direction(20,
                {599.0, 300.0, 0.9, 0.6},
                {567.0, 270.0, 0.9, 0.6},
                {569.0, 232.0, 0.9, 0.25},
                {549.0, 231.0, 0.9, 0.0},
                {536.0, 237.0, 0.9, 0.0},
                {545.0, 282.0, 0.9, 0.0},
                {552.0, 319.0, 0.9002, 0.0})
        camera_pan = 258
    elseif camera_pan == 258 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {552.0, 319.0, 0.9002, 0.0},
                {552.0, 319.0, 0.9, 0.0})
        camera_pan = 259
    elseif camera_pan == 259 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(552.0, 319.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_massagetae", 0.01)


        --BAKTRIA
    elseif camera_pan == 260 then
        scripting.game_interface:scroll_camera_with_direction(20,
                {645.0, 239.0, 0.75, -0.3},
                {610.0, 243.0, 0.75, -0.35},
                {585.0, 211.0, 0.75, -0.4},
                {597.0, 184.0, 0.75, -0.45},
                {632.0, 178.0, 0.75, -0.5})
        camera_pan = 261
    elseif camera_pan == 261 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {632.0, 178.0, 0.75, -0.5},
                {632.0, 178.0, 0.75, 0.0})
        camera_pan = 262
    elseif camera_pan == 262 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {632.0, 178.0, 0.75, 0.0},
                {604.0, 221.0, 0.75, 0.15},
                {584.0, 243.0, 0.75, 0.35})
        camera_pan = 263
    elseif camera_pan == 263 then
        scripting.game_interface:scroll_camera_with_direction(7,
                {584.0, 243.0, 0.75, 0.35},
                {584.0, 243.0, 0.75, 0.55})
        camera_pan = 264
    elseif camera_pan == 264 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {584.0, 243.0, 0.75, 0.55},
                {645.0, 239.0, 0.75, 0.0})
        camera_pan = 268
    elseif camera_pan == 268 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {645.0, 239.0, 0.75, 0.0},
                {645.0, 239.0, 0.7501, 0.0})
        camera_pan = 269
    elseif camera_pan == 269 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.75)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(645.0, 239.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_baktria", 0.01)


        --NERVII
    elseif camera_pan == 270 then
        scripting.game_interface:scroll_camera_with_direction(39,
                {156.0, 426.0, 0.9, 0.2},
                {188.0, 434.0, 0.8, 0.2},
                {194.0, 405.0, 0.8, 0.0},
                {178.0, 401.0, 0.8, 0.0},
                {155.0, 421.0, 0.8, 0.0},
                {111.0, 431.0, 0.8, 0.0},
                {93.0, 424.0, 0.8, 0.0},
        --{79.0, 419.0, 0.8, 0.0},
                {65.0, 413.0, 0.8, 0.0},
                {93.0, 396.0, 0.8, 0.0},
                {122.0, 377.0, 0.8, 0.0},
                {134.0, 414.0, 0.8, 0.0},
                {156.0, 426.0, 0.9, 0.0})
        camera_pan = 278
    elseif camera_pan == 278 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {156.0, 426.0, 0.9, 0.0},
                {156.0, 426.0, 0.9001, 0.0})
        camera_pan = 279
    elseif camera_pan == 279 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(156.0, 426.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_nervii", 0.01)


        --BOII
    elseif camera_pan == 280 then
        scripting.game_interface:scroll_camera_with_direction(22,
                {260.0, 389.0, 0.75, -0.2},
                {234.0, 411.0, 0.75, -0.1},
                {227.0, 431.0, 0.75, 0.0},
                {209.0, 439.0, 0.75, 0.0},
                {189.0, 434.0, 0.75, 0.0},
                {195.0, 406.0, 0.75, 0.0},
                {161.0, 398.0, 0.75, 0.0},
                {158.0, 376.0, 0.75, 0.0},
                {194.0, 364.0, 0.75, 0.0})
        camera_pan = 281
    elseif camera_pan == 281 then
        scripting.game_interface:scroll_camera_with_direction(2,
                {194.0, 364.0, 0.75, 0.0},
                {194.0, 364.0, 0.75, -0.1})
        camera_pan = 282
    elseif camera_pan == 282 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {194.0, 364.0, 0.75, -0.1},
                {260.0, 380.0, 0.7, -0.1},
                {303.0, 352.0, 0.8, -0.1})
        camera_pan = 283
    elseif camera_pan == 283 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {303.0, 352.0, 0.8, -0.1},
                {303.0, 352.0, 0.8, 0.0})
        camera_pan = 284
    elseif camera_pan == 284 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {303.0, 352.0, 0.8, 0.0},
                {260.0, 389.0, 0.9, 0.0})
        camera_pan = 288
    elseif camera_pan == 288 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {260.0, 389.0, 0.9, 0.0},
                {260.0, 389.0, 0.9001, 0.0})
        camera_pan = 289
    elseif camera_pan == 289 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.9)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(260.0, 389.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_boii", 0.01)


        --GALATIA
    elseif camera_pan == 290 then
        scripting.game_interface:scroll_camera_with_direction(9,
                {390.0, 271.0, 0.85, 0.2},
                {404.0, 295.0, 0.7, 0.2},
                {425.0, 280.0, 0.7, 0.0},
                {406.0, 256.0, 0.7, -0.2},
                {388.0, 250.0, 0.7, -0.2},
                {367.0, 265.0, 0.7, -0.2},
                {364.0, 282.0, 0.85, -0.2})
        camera_pan = 291
    elseif camera_pan == 291 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {364.0, 282.0, 0.85, -0.2},
                {364.0, 282.0, 0.85, 0.1})
        camera_pan = 292
    elseif camera_pan == 292 then
        scripting.game_interface:scroll_camera_with_direction(10,
                {364.0, 282.0, 0.85, 0.1},
                {404.0, 296.0, 0.85, 0.1},
                {425.0, 280.0, 0.85, 0.1},
                {407.0, 256.0, 0.85, 0.1})
        camera_pan = 293
    elseif camera_pan == 293 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {407.0, 256.0, 0.85, 0.1},
                {407.0, 256.0, 0.85, -0.1})
        camera_pan = 294
    elseif camera_pan == 294 then
        scripting.game_interface:scroll_camera_with_direction(13,
                {407.0, 256.0, 0.85, -0.1},
                {382.0, 266.0, 0.85, -0.1},
                {353.0, 283.0, 0.85, -0.1},
                {341.0, 288.0, 0.85, -0.1},
                {353.0, 299.0, 0.85, -0.1},
                {369.0, 293.0, 0.85, 0.0},
                {390.0, 271.0, 0.85, 0.0})
        camera_pan = 298
    elseif camera_pan == 298 then
        is_exiting_intro = true
        scripting.game_interface:scroll_camera_with_direction(1,
                {390.0, 271.0, 0.85, 0.0},
                {390.0, 271.0, 0.8501, 0.0})
        camera_pan = 299
    elseif camera_pan == 299 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.85)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(390.0, 271.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_galatia", 0.01)



        --LUSITANI
    elseif camera_pan == 300 then
        scripting.game_interface:make_region_visible_in_shroud("rom_lusitani", "rom_baetica_turdetania");
        scripting.game_interface:make_region_visible_in_shroud("rom_lusitani", "rom_cartaginensis_bastetania");
        scripting.game_interface:scroll_camera_with_direction(4,
                {18.3, 250.7, 0.76, 0},
                {32.6, 244.5, 0.98, 0})
        camera_pan = 301
    elseif camera_pan == 301 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {32.6, 244.5, 0.98, 0},
                {32.6, 244.5, 0.98, -0.2})
        camera_pan = 302
    elseif camera_pan == 302 then
        scripting.game_interface:scroll_camera_with_direction(28,
                {32.6, 244.5, 0.98, -0.2},
                {55.5, 219.3, 0.76, 0},
                {84.2, 220.1, 0.76, 0},
                {93.6, 244.9, 0.76, 0},
                {82.8, 274.5, 0.76, 0},
                {54.3, 273.1, 0.76, 0},
                {30.5, 259.6, 0.8, 0},
                {18.3, 250.7, 0.85, 0})
        camera_pan = 309
    elseif camera_pan == 309 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.85)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(18.3, 250.7)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_lusitani", 0.01)



        --AREVACI
    elseif camera_pan == 310 then
        scripting.game_interface:scroll_camera_with_direction(14,
                {77.0, 278.1, 0.76, 0},
                {58.1, 234.4, 0.76, 0})
        camera_pan = 311
    elseif camera_pan == 311 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {58.1, 234.4, 0.76, 0},
                {58.1, 234.4, 0.76, -0.2})
        camera_pan = 312
    elseif camera_pan == 312 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {58.1, 234.4, 0.76, -0.2},
                {19.6, 251.1, 0.76, 0})
        camera_pan = 313
    elseif camera_pan == 313 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {19.6, 251.1, 0.76, 0},
                {19.6, 251.1, 0.76, 0.2})
        camera_pan = 314
    elseif camera_pan == 314 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {19.6, 251.1, 0.76, 0.2},
                {91.9, 224.9, 0.76, 0})
        camera_pan = 315
    elseif camera_pan == 315 then
        scripting.game_interface:scroll_camera_with_direction(2,
                {91.9, 224.9, 0.76, 0},
                {91.9, 224.9, 0.76, -0.15})
        camera_pan = 316
    elseif camera_pan == 316 then
        scripting.game_interface:scroll_camera_with_direction(8,
                {91.9, 224.9, 0.76, -0.15},
                {95.4, 237.9, 0.76, 0},
                {93.5, 263.3, 0.76, 0},
                {77.0, 278.1, 0.85, 0})
        camera_pan = 319
    elseif camera_pan == 319 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.85)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(77.0, 278.1)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_arevaci", 0.01)


        -- SYRACUSE
    elseif camera_pan == 320 then
        scripting.game_interface:scroll_camera_with_direction(8,
                {224.7, 221.0, 0.76, 0},
                {215.2, 227.6, 0.76, 0},
                {197.3, 225.1, 0.76, 0},
                {185.5, 211.0, 0.76, 0})
        camera_pan = 321
    elseif camera_pan == 321 then
        scripting.game_interface:make_region_visible_in_shroud("rom_syracuse", "rom_africa_zeugitana");
        scripting.game_interface:scroll_camera_with_direction(4,
                {185.5, 211.0, 0.76, 0},
                {185.5, 211.0, 0.76, -0.2})
        camera_pan = 322
    elseif camera_pan == 322 then
        scripting.game_interface:scroll_camera_with_direction(12,
                {185.5, 211.0, 0.76, -0.2},
                {174.2, 261.1, 0.76, 0},
                {187.4, 285.7, 0.76, 0},
                {210.5, 290.4, 0.76, 0})
        camera_pan = 323
    elseif camera_pan == 323 then
        scripting.game_interface:make_region_visible_in_shroud("rom_syracuse", "rom_italia_latium");
        scripting.game_interface:scroll_camera_with_direction(4,
                {210.5, 290.4, 0.76, 0},
                {210.5, 290.4, 0.76, -0.2})
        camera_pan = 324
    elseif camera_pan == 324 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {210.5, 290.4, 0.76, -0.2},
                {307.0, 247.7, 0.76, 0})
        camera_pan = 325
    elseif camera_pan == 325 then
        scripting.game_interface:make_region_visible_in_shroud("rom_syracuse", "rom_hellas_attiki");
        scripting.game_interface:scroll_camera_with_direction(4,
                {307.0, 247.7, 0.76, 0},
                {307.0, 247.7, 0.76, -0.2})
        camera_pan = 326
    elseif camera_pan == 326 then
        scripting.game_interface:scroll_camera_with_direction(7,
                {307.0, 247.7, 0.76, -0.2},
                {293.1, 233.9, 0.76, 0},
                {260.0, 230.5, 0.76, 0},
                {224.7, 221.0, 0.76, 0})
        camera_pan = 329
    elseif camera_pan == 329 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.76)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(224.7, 221.0)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_syracuse", 0.01)



        -- TYLIS
    elseif camera_pan == 330 then
        scripting.game_interface:scroll_camera_with_direction(10,
                {345.63, 296.62, 0.76, 0},
                {301.3, 282.03, 0.76, 0})
        camera_pan = 331
    elseif camera_pan == 331 then
        scripting.game_interface:make_region_visible_in_shroud("rom_tylis", "rom_macedonia_makedonia");
        scripting.game_interface:scroll_camera_with_direction(5,
                {301.3, 282.03, 0.76, 0},
                {301.3, 282.03, 0.76, -0.2})
        camera_pan = 332
    elseif camera_pan == 332 then
        scripting.game_interface:scroll_camera_with_direction(9,
                {301.3, 282.03, 0.76, -0.2},
                {345.13, 313.82, 0.76, 0})
        camera_pan = 333
    elseif camera_pan == 333 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {345.13, 313.82, 0.76, 0},
                {345.13, 313.82, 0.76, -0.2})
        camera_pan = 334
    elseif camera_pan == 334 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {345.13, 313.82, 0.76, -0.2},
                {364.58, 281.56, 0.76, 0})
        camera_pan = 335
    elseif camera_pan == 335 then
        scripting.game_interface:make_region_visible_in_shroud("rom_tylis", "rom_bithynia_et_pontus_bythynia");
        scripting.game_interface:scroll_camera_with_direction(3,
                {364.58, 281.56, 0.76, 0},
                {364.58, 281.56, 0.76, -0.15})
        camera_pan = 336
    elseif camera_pan == 336 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {364.58, 281.56, 0.76, -0.15},
                {390.02, 271.55, 0.76, 0})
        camera_pan = 337
    elseif camera_pan == 337 then
        scripting.game_interface:make_region_visible_in_shroud("rom_tylis", "rom_galatia_et_cappadocia_galatia");
        scripting.game_interface:scroll_camera_with_direction(4,
                {390.02, 271.55, 0.76, 0},
                {390.02, 271.55, 0.76, -0.15})
        camera_pan = 338
    elseif camera_pan == 338 then
        scripting.game_interface:scroll_camera_with_direction(7,
                {390.02, 271.55, 0.76, -0.15},
                {345.63, 296.62, 0.76, 0})
        camera_pan = 339
    elseif camera_pan == 339 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.76)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(345.63, 296.62)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_tylis", 0.01)



        -- ODRYSIAN KINGDOM
    elseif camera_pan == 340 then
        scripting.game_interface:scroll_camera_with_direction(8,
                {345.03, 315.03, 0.76, 0},
                {301.3, 282.03, 0.76, 0})
        camera_pan = 341
    elseif camera_pan == 341 then
        scripting.game_interface:make_region_visible_in_shroud("rom_odryssia", "rom_macedonia_makedonia");
        scripting.game_interface:scroll_camera_with_direction(4,
                {301.3, 282.03, 0.76, 0},
                {301.3, 282.03, 0.76, -0.2})
        camera_pan = 342
    elseif camera_pan == 342 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {301.3, 282.03, 0.76, -0.2},
                {296.96, 308.86, 0.76, 0})
        camera_pan = 343
    elseif camera_pan == 343 then
        scripting.game_interface:make_region_visible_in_shroud("rom_odryssia", "rom_thracia_dardania");
        scripting.game_interface:scroll_camera_with_direction(4,
                {296.96, 308.86, 0.76, 0},
                {296.96, 308.86, 0.76, 0.2})
        camera_pan = 344
    elseif camera_pan == 344 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {296.96, 308.86, 0.76, 0.2},
                {345.26, 296.61, 0.76, 0})
        camera_pan = 345
    elseif camera_pan == 345 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {345.26, 296.61, 0.76, 0},
                {345.26, 296.61, 0.76, -0.2})
        camera_pan = 346
    elseif camera_pan == 346 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {345.26, 296.61, 0.76, -0.2},
                {315.59, 328.73, 0.76, 0})
        camera_pan = 347
    elseif camera_pan == 347 then
        scripting.game_interface:make_region_visible_in_shroud("rom_odryssia", "rom_dacia_dacia_inferior");
        scripting.game_interface:scroll_camera_with_direction(5,
                {315.59, 328.73, 0.76, 0},
                {315.59, 328.73, 0.76, 0.2})
        camera_pan = 348
    elseif camera_pan == 348 then
        scripting.game_interface:scroll_camera_with_direction(10,
                {315.59, 328.73, 0.76, 0.2},
                {345.03, 315.03, 0.76, 0})
        camera_pan = 349
    elseif camera_pan == 349 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.76)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(345.03, 315.03)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_odrysian_kingdom", 0.01)




        -- GETAE
    elseif camera_pan == 350 then
        scripting.game_interface:scroll_camera_with_direction(11,
                {316.2, 328.4, 0.76, 0},
                {405.22, 362.24, 0.76, 0})
        camera_pan = 351
    elseif camera_pan == 351 then
        scripting.game_interface:make_region_visible_in_shroud("rom_getae", "rom_ponto_caspia_cimmeria");
        scripting.game_interface:scroll_camera_with_direction(4,
                {405.22, 362.24, 0.76, 0},
                {405.22, 362.24, 0.76, -0.2})
        camera_pan = 352
    elseif camera_pan == 352 then
        scripting.game_interface:scroll_camera_with_direction(16,
                {405.22, 362.24, 0.76, -0.2},
                {341.3, 364.3, 0.76, 0},
                {306.5, 351.06, 0.76, 0},
                {297.33, 309.3, 0.76, 0})
        camera_pan = 353
    elseif camera_pan == 353 then
        scripting.game_interface:make_region_visible_in_shroud("rom_getae", "rom_thracia_dardania");
        scripting.game_interface:scroll_camera_with_direction(4,
                {297.33, 309.3, 0.76, 0},
                {297.33, 309.3, 0.76, -0.2})
        camera_pan = 354
    elseif camera_pan == 354 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {297.33, 309.3, 0.76, -0.2},
                {344.78, 313.85, 1.08, 0})
        camera_pan = 355
    elseif camera_pan == 355 then
        scripting.game_interface:make_region_visible_in_shroud("rom_getae", "rom_thracia_moesia");
        scripting.game_interface:scroll_camera_with_direction(4,
                {344.78, 313.85, 1.08, 0},
                {344.78, 313.85, 1.08, -0.2})
        camera_pan = 356
    elseif camera_pan == 356 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {344.78, 313.85, 1.08, -0.2},
                {316.2, 328.4, 0.76, 0})
        camera_pan = 359
    elseif camera_pan == 359 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.76)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(316.2, 328.4)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_getae", 0.01)



        -- ARDIAEI
    elseif camera_pan == 360 then
        scripting.game_interface:scroll_camera_with_direction(8,
                {270.37, 296.63, 0.76, 0},
                {242.70, 322.98, 0.76, 0})
        camera_pan = 361
    elseif camera_pan == 361 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {242.70, 322.98, 0.76, 0},
                {242.70, 322.98, 0.76, -0.2})
        camera_pan = 362
    elseif camera_pan == 362 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {242.70, 322.98, 0.76, -0.2},
                {263.96, 314.17, 0.76, 0})
        camera_pan = 363
    elseif camera_pan == 363 then
        scripting.game_interface:make_region_visible_in_shroud("rom_ardiaei", "rom_illyria_dalmatia");
        scripting.game_interface:scroll_camera_with_direction(4,
                {263.96, 314.17, 0.76, 0},
                {263.96, 314.17, 0.76, -0.2})
        camera_pan = 364
    elseif camera_pan == 364 then
        scripting.game_interface:make_region_visible_in_shroud("rom_ardiaei", "rom_italia_latium");
        scripting.game_interface:make_region_visible_in_shroud("rom_ardiaei", "rom_hellas_peloponnesos");
        scripting.game_interface:make_region_visible_in_shroud("rom_ardiaei", "rom_hellas_attiki");
        scripting.game_interface:scroll_camera_with_direction(27,
                {263.96, 314.17, 0.76, -0.2},
                {234.9, 308.1, 0.76, 0},
                {217.45, 288.98, 0.76, 0},
                {220.38, 273.77, 0.76, 0},
                {264.21, 243.03, 0.76, 0},
                {296.56, 242.5, 0.76, 0},
                {305.47, 263.48, 0.76, 0},
                {301.37, 274.6, 0.76, 0},
                {281.02, 291.55, 0.76, 0},
                {270.37, 296.63, 0.76, 0})
        camera_pan = 369
    elseif camera_pan == 369 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.76)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(270.37, 296.63)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_ardiaei", 0.01)



        -- ARMENIA
    elseif camera_pan == 370 then
        scripting.game_interface:scroll_camera_with_direction(10,
                {482.01, 279.89, 0.76, 0},
                {447.208374, 285.983826, 0.76, 0},
                {461.246277, 309.526306, 0.76, 0},
                {491.327576, 298.333984, 0.76, 0})
        camera_pan = 371
    elseif camera_pan == 371 then
        scripting.game_interface:scroll_camera_with_direction(2,
                {491.327576, 298.333984, 0.76, 0},
                {516.729553, 286.755676, 0.76, 0})
        camera_pan = 372
    elseif camera_pan == 372 then
        scripting.game_interface:scroll_camera_with_direction(1.5,
                {516.729553, 286.755676, 0.76, 0},
                {516.729553, 286.755676, 0.76, 0.15})
        camera_pan = 373
    elseif camera_pan == 373 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {516.729553, 286.755676, 0.76, 0.15},
                {426.485718, 278.650879, 0.76, 0})
        camera_pan = 374
    elseif camera_pan == 374 then
        scripting.game_interface:scroll_camera_with_direction(2,
                {426.485718, 278.650879, 0.76, 0},
                {426.485718, 278.650879, 0.76, 0.15})
        camera_pan = 375

    elseif camera_pan == 375 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {426.485718, 278.650879, 0.76, 0.15},
                {502.69162, 245.459763, 0.76, 0})
        camera_pan = 376

    elseif camera_pan == 376 then
        scripting.game_interface:make_region_visible_in_shroud("rom_armenia", "rom_media_magna_atropatene");
        scripting.game_interface:make_region_visible_in_shroud("rom_armenia", "rom_media_magna_media");
        scripting.game_interface:make_region_visible_in_shroud("rom_armenia", "rom_persis_elam");
        scripting.game_interface:make_region_visible_in_shroud("rom_armenia", "rom_persis_parsa");
        scripting.game_interface:scroll_camera_with_direction(8,
                {502.69162, 245.459763, 0.76, 0},
                {515.392578, 221.145355, 0.76, 0},
                {527.42511, 184.094818, 0.76, 0},
                {549.484741, 169.814941, 0.76, 0})
        camera_pan = 377
    elseif camera_pan == 377 then
        scripting.game_interface:scroll_camera_with_direction(4.25,
                {549.484741, 169.814941, 0.76, 0},
                {549.484741, 169.814941, 0.76, 0.15})
        camera_pan = 378
    elseif camera_pan == 378 then
        scripting.game_interface:scroll_camera_with_direction(8,
                {549.484741, 169.814941, 0.76, 0.15},
                {482.01, 279.89, 0.76, 0})
        camera_pan = 379
    elseif camera_pan == 379 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.76)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(482.01, 279.89)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_armenia", 0.01)



        -- MASSILIA
    elseif camera_pan == 380 then
        scripting.game_interface:make_region_visible_in_shroud("rom_massilia", "rom_corsica_et_sardinia_corsica");
        scripting.game_interface:make_region_visible_in_shroud("rom_massilia", "rom_cisalpina_liguria");
        scripting.game_interface:scroll_camera_with_direction(8.5,
                {150.406403, 306.824707, 0.76, 0},
                {172.036133, 293.137665, 0.76, 0},
                {176.476837, 328.051575, 0.76, 0})
        camera_pan = 381
    elseif camera_pan == 381 then
        scripting.game_interface:make_region_visible_in_shroud("rom_massilia", "rom_raetia_et_noricum_helvetia");
        scripting.game_interface:scroll_camera_with_direction(4.5,
                {176.476837, 328.051575, 0.76, 0},
                {168.45517, 355.067596, 0.76, 0})
        camera_pan = 382
    elseif camera_pan == 382 then
        scripting.game_interface:scroll_camera_with_direction(2,
                {168.45517, 355.067596, 0.76, 0},
                {168.45517, 355.067596, 0.76, 0.15})
        camera_pan = 383
    elseif camera_pan == 383 then
        scripting.game_interface:make_region_visible_in_shroud("rom_massilia", "rom_cisalpina_insubria");
        scripting.game_interface:scroll_camera_with_direction(3,
                {168.45517, 355.067596, 0.76, 0.15},
                {185.166992, 342.331482, 0.76, 0})
        camera_pan = 384
    elseif camera_pan == 384 then
        scripting.game_interface:scroll_camera_with_direction(2,
                {185.166992, 342.331482, 0.76, 0},
                {185.166992, 342.331482, 0.76, 0.15})
        camera_pan = 385

    elseif camera_pan == 385 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {185.166992, 342.331482, 0.76, 0.15},
                {120.993591, 314.543579, 0.76, 0})
        camera_pan = 386
    elseif camera_pan == 386 then
        scripting.game_interface:make_region_visible_in_shroud("rom_massilia", "rom_provincia_narbonensis");
        scripting.game_interface:scroll_camera_with_direction(4,
                {120.993591, 314.543579, 0.76, 0},
                {120.993591, 314.543579, 0.76, 0.15})
        camera_pan = 387
    elseif camera_pan == 387 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {120.993591, 314.543579, 0.76, 0.15},
                {150.406403, 306.824707, 0.76, 0.15})
        camera_pan = 388
    elseif camera_pan == 388 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {150.406403, 306.824707, 0.76, 0.15},
                {150.406403, 306.824707, 0.76, 0})
        camera_pan = 389
    elseif camera_pan == 389 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.76)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(150.406403, 306.824707)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_massilia", 0.01)



        -- PERGAMON
    elseif camera_pan == 390 then
        scripting.game_interface:scroll_camera_with_direction(10,
                {342.926605, 265.142883, 0.76, 0},
                {341.5896, 245.073822, 0.76, 0},
                {368.328552, 262.827209, 0.76, 0})
        camera_pan = 391
    elseif camera_pan == 391 then
        scripting.game_interface:scroll_camera_with_direction(2.5,
                {368.328552, 262.827209, 0.76, 0},
                {368.328552, 262.827209, 0.76, 0.15})
        camera_pan = 392
    elseif camera_pan == 392 then
        scripting.game_interface:scroll_camera_with_direction(2,
                {368.328552, 262.827209, 0.76, 0.15},
                {364.986206, 284.054077, 0.76, 0})
        camera_pan = 393
    elseif camera_pan == 393 then
        scripting.game_interface:scroll_camera_with_direction(1.5,
                {364.986206, 284.054077, 0.76, 0},
                {364.986206, 284.054077, 0.76, 0.15})
        camera_pan = 394
    elseif camera_pan == 394 then
        scripting.game_interface:make_region_visible_in_shroud("rom_pergamon", "rom_asia_rhodes");
        scripting.game_interface:scroll_camera_with_direction(4,
                {364.986206, 284.054077, 0.76, 0.15},
                {348.942871, 230.022034, 0.76, 0})
        camera_pan = 395

    elseif camera_pan == 395 then
        scripting.game_interface:scroll_camera_with_direction(2,
                {348.942871, 230.022034, 0.76, 0},
                {348.942871, 230.022034, 0.76, 0.15})
        camera_pan = 396
    elseif camera_pan == 396 then
        scripting.game_interface:make_region_visible_in_shroud("rom_pergamon", "rom_asia_rhodes");
        scripting.game_interface:make_region_visible_in_shroud("rom_pergamon", "rom_thracia_thracia");
        scripting.game_interface:make_region_visible_in_shroud("rom_pergamon", "rom_macedonia_makedonia");
        scripting.game_interface:scroll_camera_with_direction(8,
                {348.942871, 230.022034, 0.76, 0.15},
                {301.481262, 283.668121, 0.76, 0},
                {321.535461, 301.421509, 0.76, 0})
        camera_pan = 397
    elseif camera_pan == 397 then
        scripting.game_interface:scroll_camera_with_direction(2,
                {321.535461, 301.421509, 0.76, 0},
                {321.535461, 301.421509, 0.76, 0.15})
        camera_pan = 398
    elseif camera_pan == 398 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {321.535461, 301.421509, 0.76, 0.15},
                {342.926605, 265.142883, 0.76, 0})
        camera_pan = 399
    elseif camera_pan == 399 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.76)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(342.926605, 265.142883)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_pergamon", 0.01)



        -- CIMMERIA
    elseif camera_pan == 400 then
        scripting.game_interface:scroll_camera_with_direction(6.5,
                {413.116272, 340.401733, 0.76, 0},
                {426.485718, 336.542328, 0.76, 0},
                {439.855164, 363.558319, 0.76, 0})
        camera_pan = 401
    elseif camera_pan == 401 then
        scripting.game_interface:scroll_camera_with_direction(1.5,
                {439.855164, 363.558319, 0.76, 0},
                {439.855164, 363.558319, 0.76, 0.15})
        camera_pan = 402
    elseif camera_pan == 402 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {439.855164, 363.558319, 0.76, 0.15},
                {469.268005, 341.945526, 0.76, 0})
        camera_pan = 403
    elseif camera_pan == 403 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {469.268005, 341.945526, 0.76, 0},
                {469.268005, 341.945526, 0.76, 0.15})
        camera_pan = 404
    elseif camera_pan == 404 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {469.268005, 341.945526, 0.76, 0.15},
                {403.757629, 364.330231, 0.76, 0})
        camera_pan = 405
    elseif camera_pan == 405 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {403.757629, 364.330231, 0.76, 0},
                {403.757629, 364.330231, 0.76, 0.15})
        camera_pan = 406
    elseif camera_pan == 406 then
        scripting.game_interface:make_region_visible_in_shroud("rom_cimmeria", "rom_bithynia_et_pontus_paphlagonia");
        scripting.game_interface:make_region_visible_in_shroud("rom_cimmeria", "rom_bithynia_et_pontus_pontos");
        scripting.game_interface:scroll_camera_with_direction(8,
                {403.757629, 364.330231, 0.76, 0.15},
                {405.094574, 299.491791, 0.76, 0},
                {425.8284, 285.156952, 0.76, 0})
        camera_pan = 407
    elseif camera_pan == 407 then
        scripting.game_interface:scroll_camera_with_direction(1.5,
                {425.8284, 285.156952, 0.76, 0},
                {425.8284, 285.156952, 0.76, 0.15})
        camera_pan = 408
    elseif camera_pan == 408 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {425.8284, 285.156952, 0.76, 0.15},
                {413.116272, 340.401733, 0.76, 0})
        camera_pan = 409
    elseif camera_pan == 409 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.76)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(413.116241, 340.401733)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_cimmeria", 0.01)


        -- COLCHIS
    elseif camera_pan == 410 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_ponto_caspia_didoya");
        scripting.game_interface:scroll_camera_with_direction(8,
                {461.246277, 309.526306, 0.76, 0},
                {477.289673, 318.78894, 0.76, 0},
                {493.332977, 304.123108, 0.76, 0})
        camera_pan = 411
    elseif camera_pan == 411 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_bithynia_et_pontus_pontos");
        scripting.game_interface:scroll_camera_with_direction(4,
                {493.332977, 304.123108, 0.76, 0},
                {447.208374, 285.983765, 0.76, 0})
        camera_pan = 412
    elseif camera_pan == 412 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_bithynia_et_pontus_pontos");
        scripting.game_interface:scroll_camera_with_direction(4,
                {447.208374, 285.983765, 0.76, 0},
                {447.208374, 285.983765, 0.76, 0.15})
        camera_pan = 413
    elseif camera_pan == 413 then
        scripting.game_interface:scroll_camera_with_direction(5.5,
                {447.208374, 285.983765, 0.76, 0.15},
                {405.094574, 299.491791, 0.76, 0},
                {425.148743, 279.422791, 0.76, 0})
        camera_pan = 414
    elseif camera_pan == 414 then
        scripting.game_interface:scroll_camera_with_direction(1.5,
                {425.148743, 279.422791, 0.76, 0},
                {425.148743, 279.422791, 0.76, 0.15})
        camera_pan = 415
    elseif camera_pan == 415 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_armenia_sophene");
        scripting.game_interface:scroll_camera_with_direction(5,
                {425.148743, 279.422791, 0.76, 0.15},
                {445.871399, 264.370972, 0.76, 0},
                {481.968964, 279.036865, 0.76, 0})

        camera_pan = 416
    elseif camera_pan == 416 then
        scripting.game_interface:scroll_camera_with_direction(1.5,
                {481.968964, 279.036865, 0.76, 0},
                {481.968964, 279.036865, 0.76, 0.15})
        camera_pan = 417
    elseif camera_pan == 417 then
        scripting.game_interface:scroll_camera_with_direction(2.5,
                {481.968964, 279.036865, 0.76, 0.15},
                {461.246277, 309.526306, 0.76, 0.15})
        camera_pan = 418
    elseif camera_pan == 418 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {461.246277, 309.526306, 0.76, 0.15},
                {461.246277, 309.526306, 0.76, 0.0})
        camera_pan = 419
    elseif camera_pan == 419 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.76)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(461.246277, 309.526306)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_colchis", 0.01)
        -- KUSH
    elseif camera_pan == 420 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_ponto_caspia_didoya");
        scripting.game_interface:scroll_camera_with_direction(6,
                {400.877167, 42.273277, 0.76, 0},
                {421.485892, 69.316078, 0.76, 0})
        camera_pan = 421
    elseif camera_pan == 421 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_bithynia_et_pontus_pontos");
        scripting.game_interface:scroll_camera_with_direction(5,
                {421.485892, 69.316078, 0.76, 0},
                {390.845642, 107.271370, 0.60, 0})
        camera_pan = 422
    elseif camera_pan == 422 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_bithynia_et_pontus_pontos");
        scripting.game_interface:scroll_camera_with_direction(6,
                {390.845642, 107.271370, 0.60, 0},
                {390.845642, 114.271370, 0.76, 0.15})
        camera_pan = 423
    elseif camera_pan == 423 then
        scripting.game_interface:scroll_camera_with_direction(2,
                {390.845642, 114.271370, 0.76, 0.15},
                {432.571198, 29.9923840, 0.76, 0})
        camera_pan = 424
    elseif camera_pan == 424 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {432.571198, 29.9923840, 0.76, 0},
                {434.571198, 29.9923840, 0.76, -0.15},
                {435.571198, 29.9923840, 0.76, -0.15})
        camera_pan = 426
    elseif camera_pan == 425 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_armenia_sophene");
        scripting.game_interface:scroll_camera_with_direction(5,
                {435.571198, 29.9923840, 0.76, 0},
                {434.571198, 29.9923840, 0.76, -0.15},
                {432.571198, 29.9923840, 0.76, -0.15})
        camera_pan = 426
    elseif camera_pan == 426 then
        scripting.game_interface:scroll_camera_with_direction(2,
                {435.571198, 29.9923840, 0.76, -0.15},
                {427.455444, 114.451096, 0.76, -0.15})
        camera_pan = 427
    elseif camera_pan == 427 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {427.455444, 114.451096, 0.76, -0.15},
                {425.485444, 114.451096, 0.76, 0})
        camera_pan = 428
    elseif camera_pan == 428 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {425.485444, 114.451096, 0.76, 0},
                {400.877167, 42.273277, 0.76, 0})
        camera_pan = 429
    elseif camera_pan == 429 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.76)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(400.877167, 42.273277)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_kush", 0.01)
        -- NABATEA
    elseif camera_pan == 430 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_ponto_caspia_didoya");
        scripting.game_interface:scroll_camera_with_direction(7,
                {416.526184, 154.017838, 0.76, 0},
                {360.384979, 155.994064, 0.76, 0})
        camera_pan = 431
    elseif camera_pan == 431 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_bithynia_et_pontus_pontos");
        scripting.game_interface:scroll_camera_with_direction(4,
                {360.384979, 155.994064, 0.76, 0},
                {389.343397, 126.958656, 0.60, 0})
        camera_pan = 432
    elseif camera_pan == 432 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_bithynia_et_pontus_pontos");
        scripting.game_interface:scroll_camera_with_direction(5,
                {389.343397, 126.958656, 0.60, 0},
                {389.343397, 125.958656, 0.50, 0.15})
        camera_pan = 433
    elseif camera_pan == 433 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {389.343397, 125.958656, 0.50, 0.15},
                {427.455444, 114.451096, 0.76, 0.15})
        camera_pan = 434
    elseif camera_pan == 434 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {427.455444, 114.451096, 0.76, 0.15},
                {432.603850, 204.532944, 0.76, 0})
        camera_pan = 435
    elseif camera_pan == 435 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_armenia_sophene");
        scripting.game_interface:scroll_camera_with_direction(7,
                {432.603857, 204.532944, 0.76, 0},
                {430.603855, 204.532944, 0.76, 0.15},
                {428.603853, 204.532944, 0.76, 0.15})
        camera_pan = 436
    elseif camera_pan == 436 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {428.603850, 204.532944, 0.76, 0.15},
                {446.701477, 161.877045, 0.76, 0.15})
        camera_pan = 437
    elseif camera_pan == 437 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {446.701477, 161.877045, 0.76, 0.15},
                {446.701477, 161.877045, 0.76, 0.45})
        camera_pan = 438
    elseif camera_pan == 438 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {446.701477, 161.877045, 0.76, 0.45},
                {416.526184, 154.017838, 0.76, 0})
        camera_pan = 439
    elseif camera_pan == 439 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.76)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(416.526184, 154.017838)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_nabatea", 0.01)
        -- SABA
    elseif camera_pan == 440 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_ponto_caspia_didoya");
        scripting.game_interface:scroll_camera_with_direction(9.5,
                {492.328552, 73.835579, 0.76, 0},
                {465.48056, 81.214012, 0.95, 0.35})
        camera_pan = 441
    elseif camera_pan == 441 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_bithynia_et_pontus_pontos");
        scripting.game_interface:scroll_camera_with_direction(3,
                {465.48056, 81.214012, 0.95, 0.35},
                {480.867432, 36.744663, 0.60, 0})
        camera_pan = 442
    elseif camera_pan == 442 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_bithynia_et_pontus_pontos");
        scripting.game_interface:scroll_camera_with_direction(2,
                {480.867432, 36.744663, 0.60, 0},
                {479.867432, 36.744663, 0.60, 0})
        camera_pan = 443
    elseif camera_pan == 443 then
        scripting.game_interface:scroll_camera_with_direction(5,
                {479.867432, 36.744663, 0.60, 0},
                {442.623505, 119.562759, 0.76, 0.15})
        camera_pan = 444
    elseif camera_pan == 444 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {442.623505, 119.562759, 0.76, 0.15},
                {515.631409, 124.762375, 0.76, 0.15})
        camera_pan = 445
    elseif camera_pan == 445 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_armenia_sophene");
        scripting.game_interface:scroll_camera_with_direction(4,
                {515.631409, 124.762375, 0.76, 0.15},
                {515.631409, 124.762375, 0.95, 0.15})
        camera_pan = 446
    elseif camera_pan == 446 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {515.631409, 124.762375, 0.95, 0.15},
                {565.770633, 104.503632, 0.76, 0.15})
        camera_pan = 447
    elseif camera_pan == 447 then
        scripting.game_interface:scroll_camera_with_direction(4,
                {565.770633, 104.503632, 0.76, 0.15},
                {564.770633, 104.503632, 0.76, 0.0})
        camera_pan = 448
    elseif camera_pan == 448 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {564.770633, 104.503632, 0.76, 0.0},
                {492.328552, 73.835579, 0.76, 0.0})
        camera_pan = 449
    elseif camera_pan == 449 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.76)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(492.328552, 73.835579)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_saba", 0.01)
        -- MASAESYLI
    elseif camera_pan == 450 then
        scripting.game_interface:make_region_visible_in_shroud("rom_masaesyli", "rom_magna_graecia_sirako");
        scripting.game_interface:make_region_visible_in_shroud("rom_masaesyli", "rom_magna_graecia_lilybaion");
        scripting.game_interface:make_region_visible_in_shroud("rom_masaesyli", "rom_magna_graecia_bruttium");
        scripting.game_interface:scroll_camera_with_direction(7.5,
                {130.716553, 184.278084, 0.76, 0},
                {185.438705, 211.958694, 0.76, 0})
        camera_pan = 451
    elseif camera_pan == 451 then
        scripting.game_interface:scroll_camera_with_direction(5.5,
                {185.438705, 211.958694, 0.76, 0},
                {184.438705, 211.958694, 0.60, 0})
        camera_pan = 452
    elseif camera_pan == 452 then
        scripting.game_interface:scroll_camera_with_direction(7,
                {184.438705, 211.958694, 0.60, 0},
                {216.963501, 227.773743, 0.50, 0.15})
        camera_pan = 453
    elseif camera_pan == 453 then
        scripting.game_interface:scroll_camera_with_direction(5.5,
                {216.963501, 227.773743, 0.50, 0.15},
                {18.724873, 169.239838, 0.76, 0.15})
        camera_pan = 454
    elseif camera_pan == 454 then
        scripting.game_interface:scroll_camera_with_direction(3,
                {18.724873, 169.239838, 0.76, 0.15},
                {16.724873, 168.239838, 0.76, 0})
        camera_pan = 455
    elseif camera_pan == 455 then
        scripting.game_interface:make_region_visible_in_shroud("rom_colchis", "rom_armenia_sophene");
        scripting.game_interface:scroll_camera_with_direction(2,
                {16.724873, 168.239838, 0.76, 0},
                {17.124873, 168.139838, 0.76, 0},
                {17.724873, 168.139838, 0.76, 0})
        camera_pan = 456
    elseif camera_pan == 456 then
        scripting.game_interface:scroll_camera_with_direction(1.5,
                {17.724873, 168.239838, 0.76, 0},
                {187.407991, 153.544891, 0.76, 0.15})
        camera_pan = 457
    elseif camera_pan == 457 then
        scripting.game_interface:scroll_camera_with_direction(6,
                {187.407991, 153.544891, 0.76, 0.15},
                {184.407991, 153.544891, 0.76, 0.15})
        camera_pan = 458
    elseif camera_pan == 458 then
        scripting.game_interface:scroll_camera_with_direction(8,
                {184.407991, 153.544891, 0.76, 0.15},
                {131.716553, 184.278084, 0.76, 0})
        camera_pan = 459
    elseif camera_pan == 459 then
        panning = false
        ActivateCinematicCam(false)
        CampaignUI.SetCameraZoom(0.76)
        CampaignUI.SetCameraHeading(0.0)
        CampaignUI.SetCameraTargetInstant(130.716553, 184.278084)
        camera_pan = 0
        scripting.game_interface:add_time_trigger("rom_intro_mission_masaesyli", 0.01)

    end
end

-- IF CAMERA PAN GETS INTERRUPTED

local function OnAdviceDismissed(context)

    if camera_pan ~= 0 then
        out.ting("Advice was running, and now it's Dismissed")
        if camera_pan >= 100 and camera_pan <= 109 then
            camera_pan = 109
            OnCameraMoverFinished(context)
        elseif camera_pan >= 110 and camera_pan <= 119 then
            camera_pan = 119
            OnCameraMoverFinished(context)
        elseif camera_pan >= 120 and camera_pan <= 129 then
            camera_pan = 129
            OnCameraMoverFinished(context)
        elseif camera_pan >= 130 and camera_pan <= 139 then
            camera_pan = 139
            OnCameraMoverFinished(context)
        elseif camera_pan >= 140 and camera_pan <= 149 then
            camera_pan = 149
            OnCameraMoverFinished(context)
        elseif camera_pan >= 150 and camera_pan <= 159 then
            camera_pan = 159
            OnCameraMoverFinished(context)
        elseif camera_pan >= 160 and camera_pan <= 169 then
            camera_pan = 169
            OnCameraMoverFinished(context)
        elseif camera_pan >= 170 and camera_pan <= 179 then
            camera_pan = 179
            OnCameraMoverFinished(context)
        elseif camera_pan >= 180 and camera_pan <= 189 then
            camera_pan = 189
            OnCameraMoverFinished(context)
        elseif camera_pan >= 190 and camera_pan <= 199 then
            camera_pan = 199
            OnCameraMoverFinished(context)
        elseif camera_pan >= 200 and camera_pan <= 209 then
            camera_pan = 209
            OnCameraMoverFinished(context)
        elseif camera_pan >= 210 and camera_pan <= 219 then
            camera_pan = 219
            OnCameraMoverFinished(context)
        elseif camera_pan >= 220 and camera_pan <= 229 then
            camera_pan = 229
            OnCameraMoverFinished(context)
        elseif camera_pan >= 230 and camera_pan <= 239 then
            camera_pan = 239
            OnCameraMoverFinished(context)
        elseif camera_pan >= 240 and camera_pan <= 249 then
            camera_pan = 249
            OnCameraMoverFinished(context)
        elseif camera_pan >= 250 and camera_pan <= 259 then
            camera_pan = 259
            OnCameraMoverFinished(context)
        elseif camera_pan >= 260 and camera_pan <= 269 then
            camera_pan = 269
            OnCameraMoverFinished(context)
        elseif camera_pan >= 270 and camera_pan <= 279 then
            camera_pan = 279
            OnCameraMoverFinished(context)
        elseif camera_pan >= 280 and camera_pan <= 289 then
            camera_pan = 289
            OnCameraMoverFinished(context)
        elseif camera_pan >= 290 and camera_pan <= 299 then
            camera_pan = 299
            OnCameraMoverFinished(context)
        elseif camera_pan >= 300 and camera_pan <= 309 then
            camera_pan = 309
            OnCameraMoverFinished(context)
        elseif camera_pan >= 310 and camera_pan <= 319 then
            camera_pan = 319
            OnCameraMoverFinished(context)
        elseif camera_pan >= 320 and camera_pan <= 329 then
            camera_pan = 329
            OnCameraMoverFinished(context)
        elseif camera_pan >= 330 and camera_pan <= 339 then
            camera_pan = 339
            OnCameraMoverFinished(context)
        elseif camera_pan >= 340 and camera_pan <= 349 then
            camera_pan = 349
            OnCameraMoverFinished(context)
        elseif camera_pan >= 350 and camera_pan <= 359 then
            camera_pan = 359
            OnCameraMoverFinished(context)
        elseif camera_pan >= 360 and camera_pan <= 369 then
            camera_pan = 369
            OnCameraMoverFinished(context)
        elseif camera_pan >= 370 and camera_pan <= 379 then
            camera_pan = 379
            OnCameraMoverFinished(context)
        elseif camera_pan >= 380 and camera_pan <= 389 then
            camera_pan = 389
            OnCameraMoverFinished(context)
        elseif camera_pan >= 390 and camera_pan <= 399 then
            camera_pan = 399
            OnCameraMoverFinished(context)
        elseif camera_pan >= 400 and camera_pan <= 409 then
            camera_pan = 409
            OnCameraMoverFinished(context)
        elseif camera_pan >= 410 and camera_pan <= 419 then
            camera_pan = 419
            OnCameraMoverFinished(context)
        elseif camera_pan >= 420 and camera_pan <= 429 then
            camera_pan = 429
            OnCameraMoverFinished(context)
        elseif camera_pan >= 430 and camera_pan <= 439 then
            camera_pan = 439
            OnCameraMoverFinished(context)
        elseif camera_pan >= 440 and camera_pan <= 449 then
            camera_pan = 449
            OnCameraMoverFinished(context)
        elseif camera_pan >= 450 and camera_pan <= 459 then
            camera_pan = 459
            OnCameraMoverFinished(context)
        end
    end
end


-- TURNSTART

local function OnFactionTurnStart(context)
    player_faction = context.string
    --TIMER: CHECK IF INTRO CAMERA PAN IS RUNNING AT THE START OF THE SP GAME. IF IT'S NOT RUNNING FOR ANY REASON, GIVE THE CHAPTER OBJECTIVES
    if conditions.TurnNumber(context) == 1 and not CampaignUI.IsMultiplayer() and conditions.FactionIsLocal(context) then
        out.ting("Turn 1, single player")
        out.ting("Playing as: ".. context.string)
        scripting.game_interface:add_time_trigger("check_intro_runs", 0.01)
    end
    -- IF THERE'S NO INTRO CAMERA, THE CAMERA STARTPOS SHOULD BE SET ON TURN 1

    if conditions.FactionIsLocal(context) and conditions.TurnNumber(context) == 1 then
        if conditions.FactionName("rom_rome", context) and conditions.FactionIsHuman("rom_rome", context) and camera_pan==0 then
            out.ting("local player is Rome and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(209.0, 289.0)
        elseif conditions.FactionName("rom_carthage", context) and conditions.FactionIsHuman("rom_carthage", context) and camera_pan==0 then
            out.ting("local player is Carthage and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(186.0, 211.0)
        elseif conditions.FactionName("rom_ptolemaics", context) and conditions.FactionIsHuman("rom_ptolemaics", context) and camera_pan==0 then
            out.ting("local player is Egypt and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(357.0, 156.0)
        elseif conditions.FactionName("rom_macedon", context) and conditions.FactionIsHuman("rom_macedon", context) and camera_pan==0 then
            out.ting("local player is Macedon and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(301.0, 279.0)
        elseif conditions.FactionName("rom_iceni", context) and conditions.FactionIsHuman("rom_iceni", context) and camera_pan==0 then
            out.ting("local player is Iceni and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(112.0, 436.0)
        elseif conditions.FactionName("rom_arverni", context) and conditions.FactionIsHuman("rom_arverni", context) and camera_pan==0 then
            out.ting("local player is Arverni and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(128.0, 344.0)
        elseif conditions.FactionName("rom_suebi", context) and conditions.FactionIsHuman("rom_suebi", context) and camera_pan==0 then
            out.ting("local player is Suebi and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(228.0, 429.0)
        elseif conditions.FactionName("rom_parthia", context) and conditions.FactionIsHuman("rom_parthia", context) and camera_pan==0 then
            out.ting("local player is Parthia and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(588.0, 248.0)
        elseif conditions.FactionName("rom_pontus", context) and conditions.FactionIsHuman("rom_pontus", context) and camera_pan==0 then
            out.ting("local player is Pontus and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(425.0, 279.0)
        elseif conditions.FactionName("rom_athens", context) and conditions.FactionIsHuman("rom_athens", context) and camera_pan==0 then
            out.ting("local player is Athens and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(305.0, 248.0)
        elseif conditions.FactionName("rom_sparta", context) and conditions.FactionIsHuman("rom_sparta", context) and camera_pan==0 then
            out.ting("local player is Sparta and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(291.0, 238.0)
        elseif conditions.FactionName("rom_epirus", context) and conditions.FactionIsHuman("rom_epirus", context) and camera_pan==0 then
            out.ting("local player is Epirus and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(266.0, 275.0)
        elseif conditions.FactionName("rom_seleucid", context) and conditions.FactionIsHuman("rom_seleucid", context) and camera_pan==0 then
            out.ting("local player is Seleucid and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(415.0, 231.0)
        elseif conditions.FactionName("rom_scythia", context) and conditions.FactionIsHuman("rom_scythia", context) and camera_pan==0 then
            out.ting("local player is Scythia and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(403.0, 364.0)
        elseif conditions.FactionName("rom_roxolani", context) and conditions.FactionIsHuman("rom_roxolani", context) and camera_pan==0 then
            out.ting("local player is Roxolani and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(508.0, 426.0)
        elseif conditions.FactionName("rom_massagetae", context) and conditions.FactionIsHuman("rom_massagetae", context) and camera_pan==0 then
            out.ting("local player is Massagetae and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(552.0, 319.0)
        elseif conditions.FactionName("rom_baktria", context) and conditions.FactionIsHuman("rom_baktria", context) and camera_pan==0 then
            out.ting("local player is Baktria and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.75)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(645.0, 239.0)
        elseif conditions.FactionName("rom_nervii", context) and conditions.FactionIsHuman("rom_nervii", context) and camera_pan==0 then
            out.ting("local player is Nervii and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(156.0, 426.0)
        elseif conditions.FactionName("rom_boii", context) and conditions.FactionIsHuman("rom_boii", context) and camera_pan==0 then
            out.ting("local player is Boii and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.9)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(260.0, 389.0)
        elseif conditions.FactionName("rom_galatia", context) and conditions.FactionIsHuman("rom_galatia", context) and camera_pan==0 then
            out.ting("local player is Galatia and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(390.0, 271.0)
        elseif conditions.FactionName("rom_lusitani", context) and conditions.FactionIsHuman("rom_lusitani", context) and camera_pan==0 then
            out.ting("local player is Lusitani and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(18.3, 250.7)
        elseif conditions.FactionName("rom_arevaci", context) and conditions.FactionIsHuman("rom_arevaci", context) and camera_pan==0 then
            out.ting("local player is Arevaci and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(77.0, 278.1)
        elseif conditions.FactionName("rom_syracuse", context) and conditions.FactionIsHuman("rom_syracuse", context) and camera_pan==0 then
            out.ting("local player is Syracuse and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(224.7, 221.0)
        elseif conditions.FactionName("rom_tylis", context) and conditions.FactionIsHuman("rom_tylis", context) and camera_pan==0 then
            out.ting("local player is Tylis and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(345.63, 296.62)
        elseif conditions.FactionName("rom_odryssia", context) and conditions.FactionIsHuman("rom_odryssia", context) and camera_pan==0 then
            out.ting("local player is Odrysian Kingdom and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(345.03, 315.03)
        elseif conditions.FactionName("rom_getae", context) and conditions.FactionIsHuman("rom_getae", context) and camera_pan==0 then
            out.ting("local player is Getae and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(316.2, 328.4)
        elseif conditions.FactionName("rom_ardiaei", context) and conditions.FactionIsHuman("rom_ardiaei", context) and camera_pan==0 then
            out.ting("local player is Ardiaei and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(270.37, 296.63)
        elseif conditions.FactionName("rom_armenia", context) and conditions.FactionIsHuman("rom_armenia", context) and camera_pan==0 then
            out.ting("local player is Armenia and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(482.01, 279.89)
        elseif conditions.FactionName("rom_massilia", context) and conditions.FactionIsHuman("rom_massilia", context) and camera_pan==0 then
            out.ting("local player is Massilia and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(150.406403, 306.824707)
        elseif conditions.FactionName("rom_pergamon", context) and conditions.FactionIsHuman("rom_pergamon", context) and camera_pan==0 then
            out.ting("local player is Pergamon and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(342.926605, 265.142883)
        elseif conditions.FactionName("rom_cimmeria", context) and conditions.FactionIsHuman("rom_cimmeria", context) and camera_pan==0 then
            out.ting("local player is Cimmeria and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(413.116241, 340.401733)
        elseif conditions.FactionName("rom_colchis", context) and conditions.FactionIsHuman("rom_colchis", context) and camera_pan==0 then
            out.ting("local player is Colchis and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(461.246277, 309.526306)
        elseif conditions.FactionName("rom_kush", context) and conditions.FactionIsHuman("rom_kush", context) and camera_pan==0 then
            out.ting("local player is Kush and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(400.877167, 42.273277)
        elseif conditions.FactionName("rom_nabatea", context) and conditions.FactionIsHuman("rom_nabatea", context) and camera_pan==0 then
            out.ting("local player is Nabatea and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(416.526184, 154.017838)
        elseif conditions.FactionName("rom_saba", context) and conditions.FactionIsHuman("rom_saba", context) and camera_pan==0 then
            out.ting("local player is Saba and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(492.328552, 73.835579)
        elseif conditions.FactionName("rom_masaesyli", context) and conditions.FactionIsHuman("rom_masaesyli", context) and camera_pan==0 then
            out.ting("local player is Masaesyli and intro camera pan is not being played")
            CampaignUI.SetCameraZoom(0.85)
            CampaignUI.SetCameraHeading(0.0)
            CampaignUI.SetCameraTargetInstant(131.716553, 184.278084)
        end

    end

end

-------------------------------------------------------------------------------------------------------------------------
--
-- TIME TRIGGERS
--

local function OnTimeTrigger(context)
    -- INTRO MISSIONS, ONLY IN SP
    -- WE EITHER GIVE THEM AFTER THE INTRO CAMERA PANS, OR IF THE INTRO CAMERA PAN WAS NOT TRIGGERED FOR SOME REASON
    if (context.string == "rom_intro_mission_rome") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_rome")) then
        scripting.game_interface:trigger_custom_mission("rom_rome", "objective_rome_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_carthage") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_carthage")) then
        scripting.game_interface:trigger_custom_mission("rom_carthage", "objective_carthage_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_egypt") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_ptolemaics")) then
        scripting.game_interface:trigger_custom_mission("rom_ptolemaics", "objective_egypt_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_macedon") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_macedon")) then
        scripting.game_interface:trigger_custom_mission("rom_macedon", "objective_macedon_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_briton") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_iceni")) then
        scripting.game_interface:trigger_custom_mission("rom_iceni", "objective_briton_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_gaul") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_arverni")) then
        scripting.game_interface:trigger_custom_mission("rom_arverni", "objective_gaul_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_germanic") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_suebi")) then
        scripting.game_interface:trigger_custom_mission("rom_suebi", "objective_german_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_parthia") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_parthia")) then
        scripting.game_interface:trigger_custom_mission("rom_parthia", "objective_parthia_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_pontus") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_pontus")) then
        scripting.game_interface:trigger_custom_mission("rom_pontus", "objective_pontus_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_athens") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_athens")) then
        scripting.game_interface:trigger_custom_mission("rom_athens", "objective_athens_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_sparta") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_sparta")) then
        scripting.game_interface:trigger_custom_mission("rom_sparta", "objective_sparta_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_epirus") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_epirus")) then
        scripting.game_interface:trigger_custom_mission("rom_epirus", "objective_epirus_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_seleucid") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_seleucid")) then
        scripting.game_interface:trigger_custom_mission("rom_seleucid", "objective_seleucid_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_scythia") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_scythia")) then
        scripting.game_interface:trigger_custom_mission("rom_scythia", "objective_scythia_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_roxolani") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_roxolani")) then
        scripting.game_interface:trigger_custom_mission("rom_roxolani", "objective_roxolani_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_massagetae") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_massagetae")) then
        scripting.game_interface:trigger_custom_mission("rom_massagetae", "objective_massagetae_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_baktria") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_baktria")) then
        scripting.game_interface:trigger_custom_mission("rom_baktria", "objective_baktria_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_nervii") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_nervii")) then
        scripting.game_interface:trigger_custom_mission("rom_nervii", "objective_nervii_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_boii") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_boii")) then
        scripting.game_interface:trigger_custom_mission("rom_boii", "objective_boii_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_galatia") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_galatia")) then
        scripting.game_interface:trigger_custom_mission("rom_galatia", "objective_galatia_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_syracuse") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_syracuse")) then
        scripting.game_interface:trigger_custom_mission("rom_syracuse", "objective_syracuse_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_lusitani") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_lusitani")) then
        scripting.game_interface:trigger_custom_mission("rom_lusitani", "objective_lusitani_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_arevaci") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_arevaci")) then
        scripting.game_interface:trigger_custom_mission("rom_arevaci", "objective_arevaci_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_tylis") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_tylis")) then
        scripting.game_interface:trigger_custom_mission("rom_tylis", "objective_tylis_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_odrysian_kingdom") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_odryssia")) then
        scripting.game_interface:trigger_custom_mission("rom_odryssia", "objective_odrysian_kingdom_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_getae") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_getae")) then
        scripting.game_interface:trigger_custom_mission("rom_getae", "objective_getae_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_ardiaei") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_ardiaei")) then
        scripting.game_interface:trigger_custom_mission("rom_ardiaei", "objective_ardiaei_1_primary")
        scripting.game_interface:show_message_event("custom_event_9", 0, 0)
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_armenia") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_armenia")) then
        scripting.game_interface:trigger_custom_mission("rom_armenia", "objective_armenia_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_massilia") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_massilia")) then
        scripting.game_interface:trigger_custom_mission("rom_massilia", "objective_massilia_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_pergamon") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_pergamon")) then
        scripting.game_interface:trigger_custom_mission("rom_pergamon", "objective_pergamon_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_cimmeria") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_cimmeria")) then
        scripting.game_interface:trigger_custom_mission("rom_cimmeria", "objective_cimmeria_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_colchis") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_colchis")) then
        scripting.game_interface:trigger_custom_mission("rom_colchis", "objective_colchis_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_kush") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_kush")) then
        scripting.game_interface:trigger_custom_mission("rom_kush", "objective_kush_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_nabatea") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_nabatea")) then
        scripting.game_interface:trigger_custom_mission("rom_nabatea", "objective_nabatea_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_saba") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_saba")) then
        scripting.game_interface:trigger_custom_mission("rom_saba", "objective_saba_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    elseif (context.string == "rom_intro_mission_masaesyli") or ((context.string == "check_intro_runs") and (intro_advice_shown == false) and (player_faction == "rom_masaesyli")) then
        scripting.game_interface:trigger_custom_mission("rom_masaesyli", "objective_masaesyli_1_primary")
        out.ting("DEBUG: Intro advice shown:"..tostring(intro_advice_shown))
    end
end

-------------------------------------------------------------------------------------------------------------------------
--
-- SET UP THE ESCAPE KEY TO CIRCUMVENT PLAYER INTERACTION LOCKING
--

function OnKeyPressed(key, is_key_up)
    if is_key_up == true then
        out.ting("Key pressed up")
        if key == "ESCAPE" or key == "SPACE" then
            out.ting("Escape or space pressed")
            if camera_pan ~= 0 and is_exiting_intro == false then
                is_exiting_intro = true
                out.ting("Escape or space pressed, Cancelling the advice")
                scripting.game_interface:stop_camera()
                scripting.game_interface:dismiss_advice()
                --		OnCameraMoverFinished(context)
            end
        end
    end
end

--
-- IF CLICKING ON THE CLOSE ADVISOR BUTTON DURING THE PAN, THEN END WITHOUT LOCKING THE CAMERA
--

local function OnComponentLClickUp(context)
    if conditions.IsComponentType("button_close", context) and (camera_pan>0) and is_exiting_intro == false then
        is_exiting_intro = true
        out.ting("Cancelling the advice")
        scripting.game_interface:stop_camera()
        ActivateCinematicCam(false)		--if I don't call this, the game will freeze if someone presses the X button and then hits ESC. No idea why, and this solution shouldn't work, but it works. Magic...
        --	scripting.game_interface:dismiss_advice()
        --	OnCameraMoverFinished(context)
    end
end


--------------------------------------------------------------------------------------------------------------------
-- Add event callbacks
-- For a list of all events supported create a "documentation" directory in your empire directory, run a debug build of the game and see
-- the events.txt file
--------------------------------------------------------------------------------------------------------------------

scripting.AddEventCallBack("NewCampaignStarted", OnNewCampaignStarted)
scripting.AddEventCallBack("UICreated", OnUICreated)

scripting.AddEventCallBack("AdviceIssued", OnAdviceIssued)
scripting.AddEventCallBack("AdviceDismissed", OnAdviceDismissed)
scripting.AddEventCallBack("CameraMoverFinished", OnCameraMoverFinished)
scripting.AddEventCallBack("FactionTurnStart", OnFactionTurnStart)
scripting.AddEventCallBack("ComponentLClickUp", OnComponentLClickUp)

scripting.AddEventCallBack("TimeTrigger", OnTimeTrigger)

logG = require('script._lib.lib_logging').new_logger("main_rome_scriptingG", "main_rome_scriptingG.log", "TRACE")
logR = require('script._lib.lib_logging').new_logger("main_rome_scriptingR", "main_rome_scriptingR.log", "TRACE")
logE = require('script._lib.lib_logging').new_logger("main_rome_scriptingE", "main_rome_scriptingE.log", "TRACE")

logG:pretty_table(_G)
logR:pretty_table(debug.getregistry())
logE:pretty_table(getfenv())
