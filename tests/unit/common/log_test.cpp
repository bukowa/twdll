#include <doctest.h>
#include "common/log.h"
#include <cstdio>
#include <fstream>
#include <string>
#include <vector>
#include <thread>
#include <atomic>
#include <optional>

namespace {

size_t get_file_size(const char* path) {
    std::ifstream in(path, std::ifstream::ate | std::ifstream::binary);
    return in.is_open() ? static_cast<size_t>(in.tellg()) : 0;
}

std::string read_entire_file(const char* path) {
    std::ifstream in(path, std::ios::binary);
    if (!in.is_open()) return "";
    return std::string((std::istreambuf_iterator<char>(in)),
                        std::istreambuf_iterator<char>());
}

} // namespace

TEST_SUITE("src/common/log.cpp: Adversarial & Edge Case Tests") {

    TEST_CASE("Standard formatting: writes formatted message to twdll.log with newline") {
        const char* test_file = "twdll.log";
        std::remove(test_file);

        set_logging_enabled(true);
        Log("Test message: %s %d", "hello", 42);

        std::string content = read_entire_file(test_file);
        CHECK(content.rfind("\xEF\xBB\xBF") == std::string::npos); // Ensure no UTF-8 BOM
        CHECK(content.find("Test message: hello 42") != std::string::npos);
        CHECK_FALSE(content.empty());
        CHECK(content.back() == '\n');

        set_logging_enabled(false);
    }

    TEST_CASE("Crash resilience: nullptr format string must not crash process") {
        set_logging_enabled(true);
        // In unhardened code, vfprintf(f, nullptr) causes crash 0xC0000005.
        // The logger MUST gracefully guard against nullptr format strings.
        CHECK_NOTHROW(Log(static_cast<const char*>(nullptr)));
        set_logging_enabled(false);
    }

    TEST_CASE("Crash resilience: null argument in %s specifier") {
        set_logging_enabled(true);
        const char* null_str = nullptr;
        CHECK_NOTHROW(Log("Test null param: %s", null_str));
        set_logging_enabled(false);
    }

    TEST_CASE("Buffer boundaries: massive string payload (32 KB)") {
        set_logging_enabled(true);
        const char* test_file = "twdll.log";
        std::remove(test_file);

        std::string huge_string(32 * 1024, 'A');
        huge_string += "_END_MARKER";

        CHECK_NOTHROW(Log("HUGE: %s", huge_string.c_str()));

        std::string content = read_entire_file(test_file);
        CHECK(content.find("HUGE: ") != std::string::npos);
        CHECK(content.find("_END_MARKER") != std::string::npos);
        CHECK_FALSE(content.empty());
        CHECK(content.back() == '\n');

        set_logging_enabled(false);
    }

    TEST_CASE("Physical disk guarantee: zero bytes written when disabled") {
        const char* test_file = "twdll.log";
        std::remove(test_file);

        set_logging_enabled(false);

        // Attempt intensive logging while disabled
        for (int i = 0; i < 100; ++i) {
            Log("This must NEVER be written to disk: %d", i);
        }

        CHECK_EQ(get_file_size(test_file), 0);
    }

    TEST_CASE("Concurrency stress: parallel logging under state toggling") {
        const char* test_file = "twdll.log";
        std::remove(test_file);

        std::atomic<bool> stop_flag{false};
        std::atomic<int> logged_count{0};

        // Thread toggling logging enabled state rapidly
        std::thread toggle_thread([&]() {
            bool state = false;
            while (!stop_flag.load(std::memory_order_relaxed)) {
                set_logging_enabled(state);
                state = !state;
                std::this_thread::yield();
            }
        });

        // 4 worker threads hammering Log() simultaneously
        auto worker = [&](int thread_id) {
            for (int i = 0; i < 200; ++i) {
                Log("[Thread %d] Stress message %d", thread_id, i);
                logged_count.fetch_add(1, std::memory_order_relaxed);
            }
        };

        std::vector<std::thread> workers;
        for (int i = 0; i < 4; ++i) {
            workers.emplace_back(worker, i);
        }

        for (auto& w : workers) {
            w.join();
        }

        stop_flag.store(true);
        toggle_thread.join();

        set_logging_enabled(false);

        // If threads corrupted memory or deadlocked, we'd never reach here.
        CHECK_EQ(logged_count.load(), 800);
    }
}

TEST_SUITE("src/common/log.cpp: Modular Multi-Channel & Severity Hierarchy Tests") {

    TEST_CASE("parse_log_level: parses severity strings case-insensitively with aliases") {
        CHECK_EQ(parse_log_level("none"), LogLevel::None);
        CHECK_EQ(parse_log_level("NONE"), LogLevel::None);
        CHECK_EQ(parse_log_level("fatal"), LogLevel::Fatal);
        CHECK_EQ(parse_log_level("FATAL"), LogLevel::Fatal);
        CHECK_EQ(parse_log_level("error"), LogLevel::Error);
        CHECK_EQ(parse_log_level("warn"), LogLevel::Warn);
        CHECK_EQ(parse_log_level("warning"), LogLevel::Warn);
        CHECK_EQ(parse_log_level("WARNING"), LogLevel::Warn);
        CHECK_EQ(parse_log_level("info"), LogLevel::Info);
        CHECK_EQ(parse_log_level("INFO"), LogLevel::Info);
        CHECK_EQ(parse_log_level("debug"), LogLevel::Debug);
        CHECK_EQ(parse_log_level("DEBUG"), LogLevel::Debug);
        CHECK_EQ(parse_log_level("trace"), LogLevel::Trace);
        CHECK_EQ(parse_log_level("verbose"), LogLevel::Trace);
        CHECK_EQ(parse_log_level("unknown_invalid_level"), LogLevel::Info); // safe fallback
        CHECK_EQ(parse_log_level(nullptr), LogLevel::Info);
    }

    TEST_CASE("log_level_to_string: converts levels to canonical lowercase names") {
        CHECK_EQ(std::string(log_level_to_string(LogLevel::None)), "none");
        CHECK_EQ(std::string(log_level_to_string(LogLevel::Fatal)), "fatal");
        CHECK_EQ(std::string(log_level_to_string(LogLevel::Error)), "error");
        CHECK_EQ(std::string(log_level_to_string(LogLevel::Warn)), "warn");
        CHECK_EQ(std::string(log_level_to_string(LogLevel::Info)), "info");
        CHECK_EQ(std::string(log_level_to_string(LogLevel::Debug)), "debug");
        CHECK_EQ(std::string(log_level_to_string(LogLevel::Trace)), "trace");
    }

    TEST_CASE("Channel registry & level inheritance") {
        set_logging_enabled(true);

        set_root_log_level(LogLevel::Info);
        CHECK_EQ(get_root_log_level(), LogLevel::Info);

        // Child channels inherit root level when no override is set
        CHECK_EQ(get_channel_effective_level("MyMod"), LogLevel::Info);
        CHECK(is_channel_level_enabled("MyMod", LogLevel::Info));
        CHECK(is_channel_level_enabled("MyMod", LogLevel::Warn));
        CHECK_FALSE(is_channel_level_enabled("MyMod", LogLevel::Debug));

        // When root level changes, inherited channels follow
        set_root_log_level(LogLevel::Warn);
        CHECK_EQ(get_channel_effective_level("MyMod"), LogLevel::Warn);
        CHECK_FALSE(is_channel_level_enabled("MyMod", LogLevel::Info));

        // Channel override decouples from root
        set_channel_log_level("MyMod", LogLevel::Debug);
        CHECK_EQ(get_channel_effective_level("MyMod"), LogLevel::Debug);
        CHECK(is_channel_level_enabled("MyMod", LogLevel::Debug));
        CHECK(is_channel_level_enabled("MyMod", LogLevel::Info));

        // Root level changes do not affect overridden channel
        set_root_log_level(LogLevel::Error);
        CHECK_EQ(get_channel_effective_level("MyMod"), LogLevel::Debug);

        // Reset override restores inheritance
        set_channel_log_level("MyMod", std::nullopt);
        CHECK_EQ(get_channel_effective_level("MyMod"), LogLevel::Error);

        // CORE channel isolation
        CHECK_EQ(get_channel_effective_level("CORE"), LogLevel::Info);
        set_channel_log_level("CORE", LogLevel::Warn);
        CHECK_EQ(get_channel_effective_level("CORE"), LogLevel::Warn);

        set_logging_enabled(false);
        // When master logging is disabled, all channels report false for enabled
        CHECK_FALSE(is_channel_level_enabled("MyMod", LogLevel::Fatal));
    }

    TEST_CASE("Log formatting with channel tags and severity in twdll.log") {
        const char* test_file = "twdll.log";
        std::remove(test_file);

        set_logging_enabled(true);
        set_root_log_level(LogLevel::Debug);

        LogChannel("SupplyMod", LogLevel::Info, "Units resupplied: 120");
        LogChannel("BattleAI", LogLevel::Warn, "Flank exposed on unit 5");

        std::string content = read_entire_file(test_file);
        CHECK(content.find("[INFO]  [SupplyMod] Units resupplied: 120") != std::string::npos);
        CHECK(content.find("[WARN]  [BattleAI] Flank exposed on unit 5") != std::string::npos);

        set_logging_enabled(false);
    }
}

