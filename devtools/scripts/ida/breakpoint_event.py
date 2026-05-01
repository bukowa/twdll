# --- FINAL SCRIPT (v6) ---
#
# RULE: Break on everything, EXCEPT for valid strings found in the ignore_list.
# FIX:  Decodes the byte string returned by IDA into a regular string before
#       comparison. This handles the b'...' mismatch.
#

ignore_list = {
    "AdviceDismissed", "AdviceFinishedTrigger", "AdviceIssued", "AdviceSuperseded",
    "AreaCameraEntered", "AreaCameraExited", "AreaEntered", "AreaExited",
    "ArmyBribeAttemptFailure", "ArmySabotageAttemptFailure", "ArmySabotageAttemptSuccess",
    "AssassinationAttemptCriticalSuccess", "AssassinationAttemptFailure", "AssassinationAttemptSuccess",
    "BattleBoardingActionCommenced", "BattleBoardingShip", "BattleCommandingShipRouts",
    "BattleCommandingUnitRouts", "BattleCompleted", "BattleConflictPhaseCommenced",
    "BattleDeploymentPhaseCommenced", "BattleFortPlazaCaptureCommenced", "BattleShipAttacksEnemyShip",
    "BattleShipCaughtFire", "BattleShipMagazineExplosion", "BattleShipRouts",
    "BattleShipRunAground", "BattleShipSailingIntoWind", "BattleShipSurrendered",
    "BattleUnitAttacksBuilding", "BattleUnitAttacksEnemyUnit", "BattleUnitAttacksWalls",
    "BattleUnitCapturesBuilding", "BattleUnitDestroysBuilding", "BattleUnitRouts",
    "BattleUnitUsingBuilding", "BattleUnitUsingWall", "BuildingCardSelected",
    "BuildingCompleted", "BuildingConstructionIssuedByPlayer", "BuildingInfoPanelOpenedCampaign",
    "CameraMoverCancelled", "CameraMoverFinished", "CampaignArmiesMerge",
    "CampaignBuildingDamaged", "CampaignCoastalAssaultOnCharacter", "CampaignCoastalAssaultOnGarrison",
    "CampaignEffectsBundleAwarded", "CampaignSettlementAttacked", "CharacterAttacksAlly",
    "CharacterBecomesFactionLeader", "CharacterBesiegesSettlement", "CharacterBlockadedPort",
    "CharacterBrokePortBlockade", "CharacterBuildingCompleted", "CharacterCanLiberate",
    "CharacterCandidateBecomesMinister", "CharacterCharacterTargetAction", "CharacterComesOfAge",
    "CharacterCompletedBattle", "CharacterCreated", "CharacterDamagedByDisaster",
    "CharacterDeselected", "CharacterDiscovered", "CharacterDisembarksNavy",
    "CharacterEmbarksNavy", "CharacterEntersAttritionalArea", "CharacterEntersGarrison",
    "CharacterFactionCompletesResearch", "CharacterFamilyRelationDied", "CharacterGarrisonTargetAction",
    "CharacterGeneralDiedInBattle", "CharacterInfoPanelOpened", "CharacterLeavesGarrison",
    "CharacterLootedSettlement", "CharacterMarriage", "CharacterMilitaryForceTraditionPointAllocated",
    "CharacterParticipatedAsSecondaryGeneralInBattle", "CharacterPerformsActionAgainstFriendlyTarget",
    "CharacterPoliticalAction", "CharacterPoliticalActionPoliticalMariage", "CharacterPoliticalAdoption",
    "CharacterPoliticalAssassination", "CharacterPoliticalBribe", "CharacterPoliticalDivorce",
    "CharacterPoliticalEmbezzleFunds", "CharacterPoliticalEntice", "CharacterPoliticalFlirt",
    "CharacterPoliticalGatherSupport", "CharacterPoliticalInsult", "CharacterPoliticalOrganizeGames",
    "CharacterPoliticalPartyProvoke", "CharacterPoliticalPartyPurge", "CharacterPoliticalPartySecureLoyalty",
    "CharacterPoliticalPraise", "CharacterPoliticalPromotion", "CharacterPoliticalProvoke",
    "CharacterPoliticalRumours", "CharacterPoliticalSecureLoyalty", "CharacterPoliticalSendDiplomat",
    "CharacterPoliticalSendEmissary", "CharacterPoliticalSendGift", "CharacterPoliticalSuicide",
    "CharacterPostBattleEnslave", "CharacterPostBattleRelease", "CharacterPostBattleSlaughter",
    "CharacterPromoted", "CharacterRankUp", "CharacterRankUpNeedsAncillary",
    "CharacterRelativeKilled", "CharacterSelected", "CharacterSettlementBesieged",
    "CharacterSettlementBlockaded", "CharacterSkillPointAllocated", "CharacterSuccessfulArmyBribe",
    "CharacterSuccessfulConvert", "CharacterSuccessfulDemoralise", "CharacterSuccessfulInciteRevolt",
    "CharacterSurvivesAssassinationAttempt", "CharacterTurnEnd", "CharacterTurnStart",
    "CharacterTurnStartCarthage", "CharacterTurnStartRome", "CharacterWoundedInAssassinationAttempt",
    "ClanBecomesVassal", "ComponentCreated", "ComponentLClickUp", "ComponentMouseOn",
    "ComponentMoved", "ConvertAttemptFailure", "CustomMission", "DemoraliseAttemptFailure",
    "DuelDemanded", "DummyEvent", "EncylopediaEntryRequested", "EventMessageOpenedBattle",
    "EventMessageOpenedCampaign", "FactionAboutToEndTurn", "FactionBattleDefeat",
    "FactionBattleVictory", "FactionBecomesLiberationProtectorate", "FactionBecomesLiberationVassal",
    "FactionBecomesShogun", "FactionBecomesWorldLeader", "FactionBeginTurnPhaseNormal",
    "FactionCapturesKyoto", "FactionCapturesWorldCapital", "FactionCivilWarEnd",
    "FactionEncountersOtherFaction", "FactionFameLevelUp", "FactionGovernmentTypeChanged",
    "FactionLeaderDeclaresWar", "FactionLeaderIssuesEdict", "FactionLeaderSignsPeaceTreaty",
    "FactionPoliticsGovernmentActionTriggered", "FactionPoliticsGovernmentTypeChanged",
    "FactionRoundStart", "FactionSecessionEnd", "FactionSubjugatesOtherFaction",
    "FactionTurnEnd", "FactionTurnStart", "FirstTickAfterNewCampaignStarted",
    "FirstTickAfterWorldCreated", "ForceAdoptsStance", "FortSelected",
    "FrontendScreenTransition", "GarrisonAttackedEvent", "GarrisonOccupiedEvent",
    "GarrisonResidenceCaptured", "GovernorshipTaxRateChanged", "HistoricBattleEvent",
    "HistoricalCharacters", "HistoricalEvents", "HudRefresh", "InciteRevoltAttemptFailure",
    "IncomingMessage", "LandTradeRouteRaided", "LoadingGame", "LoadingScreenDismissed",
    "LocationEntered", "LocationUnveiled", "MPLobbyChatCreated", "MapIconMoved",
    "MissionCancelled", "MissionCheckAssassination", "MissionCheckBlockadePort",
    "MissionCheckBuild", "MissionCheckCaptureCity", "MissionCheckDuel",
    "MissionCheckEngageCharacter", "MissionCheckEngageFaction", "MissionCheckGainMilitaryAccess",
    "MissionCheckMakeAlliance", "MissionCheckMakeTradeAgreement", "MissionCheckRecruit",
    "MissionCheckResearch", "MissionCheckSpyOnCity", "MissionEvaluateAssassination",
    "MissionEvaluateBlockadePort", "MissionEvaluateBuild", "MissionEvaluateCaptureCity",
    "MissionEvaluateDuel", "MissionEvaluateEngageCharacter", "MissionEvaluateEngageFaction",
    "MissionEvaluateGainMilitaryAccess", "MissionEvaluateMakeAlliance", "MissionEvaluateMakeTradeAgreement",
    "MissionEvaluateRecruit", "MissionEvaluateResearch", "MissionEvaluateSpyOnCity",
    "MissionFailed", "MissionIssued", "MissionNearingExpiry", "MissionSucceeded",
    "ModelCreated", "MovementPointsExhausted", "MultiTurnMove", "NewCampaignStarted",
    "NewSession", "PanelAdviceRequestedBattle", "PanelAdviceRequestedCampaign",
    "PanelClosedBattle", "PanelClosedCampaign", "PanelOpenedBattle", "PanelOpenedCampaign",
    "PendingBankruptcy", "PendingBattle", "PositiveDiplomaticEvent", "PreBattle",
    "RecruitmentItemIssuedByPlayer", "RegionChangedFaction", "RegionGainedDevlopmentPoint",
    "RegionIssuesDemands", "RegionRebels", "RegionRiots", "RegionSelected",
    "RegionStrikes", "RegionTurnEnd", "RegionTurnStart", "ResearchCompleted",
    "ResearchStarted", "SabotageAttemptFailure", "SabotageAttemptSuccess",
    "SavingGame", "ScriptedAgentCreated", "ScriptedAgentCreationFailed",
    "ScriptedCharacterUnhidden", "ScriptedCharacterUnhiddenFailed", "ScriptedForceCreated",
    "SeaTradeRouteRaided", "SettlementDeselected", "SettlementOccupied",
    "SettlementSelected", "ShortcutTriggered", "SiegeLifted", "SlotOpens",
    "SlotRoundStart", "SlotSelected", "SlotTurnStart", "StartRegionPopupVisible",
    "StartRegionSelected", "TechnologyInfoPanelOpenedCampaign", "TestEvent",
    "TimeTrigger", "TooltipAdvice", "TouchUsed", "TradeLinkEstablished",
    "TradeNodeConnected", "TradeRouteEstablished", "UICreated", "UIDestroyed",
    "UngarrisonedFort", "UnitCompletedBattle", "UnitCreated", "UnitSelectedCampaign",
    "UnitTrained", "UnitTurnEnd", "VictoryConditionFailed", "VictoryConditionMet",
    "WorldCreated", "historical_events"
}

def main():
    try:
        string_ptr = idc.get_reg_value("eax")

        if not string_ptr:
            print("IDA Breakpoint: EAX is NULL. Breaking.")
            return True

        if not idc.is_mapped(string_ptr):
            print(f"IDA Breakpoint: Pointer {hex(string_ptr)} is not to mapped memory. Breaking.")
            return True

        # Use integer value 3 for wide-strings, 0 for C-strings.
        current_string_bytes = idc.get_strlit_contents(string_ptr, -1, 3) 
        if current_string_bytes is None:
            current_string_bytes = idc.get_strlit_contents(string_ptr, -1, 0)

        if current_string_bytes is None:
            print(f"IDA Breakpoint: Could not read a valid string from {hex(string_ptr)}. Breaking.")
            return True

        # --- THIS IS THE FIX ---
        # Decode the byte string (b'...') into a regular string ('...')
        try:
            current_string = current_string_bytes.decode('utf-8')
        except (UnicodeDecodeError, AttributeError):
            print(f"IDA Breakpoint: Could not decode bytes from {hex(string_ptr)}. Breaking.")
            return True
        # --- END OF FIX ---

        # Now the comparison will work correctly.
        if current_string in ignore_list:
            return False # This is the ONLY way to NOT break.
        
        print(f"IDA Breakpoint: New/Unknown string found -> '{current_string}'. Breaking.")
        return True

    except Exception as e:
        print(f"IDA Breakpoint: An unexpected exception occurred: {e}. Breaking.")
        return True

return main()