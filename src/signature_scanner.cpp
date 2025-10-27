#include "signature_scanner.h"

// Simple helper to check if a byte matches a pattern byte (considering wildcards)
static bool byte_matches(const unsigned char byte, const unsigned char pattern_byte, const unsigned char mask_byte) {
    return (byte & mask_byte) == (pattern_byte & mask_byte);
}

// Signature scanning function
uintptr_t find_signature(uintptr_t base_address, size_t size, const char* signature_str) {
    // Convert signature string to byte array and generate mask
    std::vector<unsigned char> signature_bytes;
    std::vector<unsigned char> mask_bytes;

    std::stringstream ss_sig(signature_str);
    std::string byte_hex;
    while (ss_sig >> byte_hex) {
        if (byte_hex == "??") {
            signature_bytes.push_back(0x00); // Wildcard, actual value doesn't matter
            mask_bytes.push_back(0x00);     // Mask out this byte
        } else {
            signature_bytes.push_back(static_cast<unsigned char>(std::stoul(byte_hex, nullptr, 16)));
            mask_bytes.push_back(0xFF);     // Match this byte exactly
        }
    }

    // Perform the scan
    for (size_t i = 0; i < size - signature_bytes.size(); ++i) {
        bool found = true;
        for (size_t j = 0; j < signature_bytes.size(); ++j) {
            if (!byte_matches(*(unsigned char*)(base_address + i + j), signature_bytes[j], mask_bytes[j])) {
                found = false;
                break;
            }
        }
        if (found) {
            return base_address + i;
        }
    }
    return 0; // Not found
}
