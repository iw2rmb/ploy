#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 1 ]; then
  cat >&2 <<'USAGE'
Usage:
  tools/roadmap/verify_done.sh <phase-yaml> [<phase-yaml> ...]

Behavior:
  - For each phase with done=true:
    - fails if any item has done!=true
    - fails if any unresolved reviews.gaps exist (phase or item level)
    - fails if the phase index evidence entry is missing or unchecked in sibling index.md

Evidence marker convention:
  - Marker text: evidence:<phase-basename-without-.yaml>
  - Marker location: checklist entry in roadmap/<subject>/index.md
USAGE
  exit 2
fi

if ! command -v ruby >/dev/null 2>&1; then
  echo "error: ruby is required for YAML parsing" >&2
  exit 2
fi

ruby - "$@" <<'RUBY'
require "yaml"

class Verifier
  def initialize(paths)
    @paths = paths
    @failures = []
    @checked = 0
  end

  def run
    @paths.each { |path| verify_phase(path) }

    if @failures.any?
      @failures.each { |line| warn(line) }
      warn("roadmap verification failed")
      return 1
    end

    puts("roadmap verification passed (#{@checked} phase#{@checked == 1 ? "" : "s"} checked)")
    0
  end

  private

  def verify_phase(path)
    unless File.file?(path)
      @failures << "error: missing phase file: #{path}"
      return
    end

    data = YAML.load_file(path)
    unless data.is_a?(Hash)
      @failures << "error: invalid YAML object at #{path}"
      return
    end

    done = data["done"] == true
    return unless done

    @checked += 1
    phase_name = File.basename(path, ".yaml")
    unresolved = []
    incomplete_items = []

    unresolved.concat(find_unresolved_reviews(data["reviews"], "phase"))

    items = data["items"]
    if items.is_a?(Array)
      items.each_with_index do |item, idx|
        next unless item.is_a?(Hash)

        label = item["label"].to_s.strip
        marker = label.empty? ? "item[#{idx}]" : "item #{label}"
        incomplete_items << marker unless item["done"] == true
        unresolved.concat(find_unresolved_reviews(item["reviews"], marker))
      end
    end

    if incomplete_items.any?
      @failures << "error: phase marked done but contains incomplete items in #{path}"
      incomplete_items.each { |label| @failures << "  - #{label} has done!=true" }
    end

    if unresolved.any?
      @failures << "error: unresolved reviews.gaps in #{path}"
      unresolved.each { |msg| @failures << "  - #{msg}" }
    end

    index_path = File.join(File.dirname(path), "index.md")
    evidence_marker = "evidence:#{phase_name}"

    unless File.file?(index_path)
      @failures << "error: missing roadmap index for #{path} (expected #{index_path})"
      return
    end

    index_text = File.read(index_path)
    lines_with_marker = index_text.each_line.select { |line| line.include?(evidence_marker) }
    if lines_with_marker.empty?
      @failures << "error: missing evidence marker '#{evidence_marker}' in #{index_path}"
      return
    end

    has_checked_entry = lines_with_marker.any? { |line| line.match?(/^\s*-\s*\[[xX]\]/) }
    unless has_checked_entry
      @failures << "error: evidence marker '#{evidence_marker}' is present but unchecked in #{index_path}"
    end
  rescue Psych::SyntaxError => e
    @failures << "error: YAML parse failure in #{path}: #{e.message.lines.first.to_s.strip}"
  end

  def find_unresolved_reviews(reviews, scope)
    return [] unless reviews.is_a?(Array)

    unresolved = []
    reviews.each_with_index do |review, idx|
      next unless review.is_a?(Hash)

      gaps = review["gaps"]
      has_gaps = gaps.is_a?(Array) && !gaps.empty?
      commit = review["commit"].to_s.strip
      has_commit = !commit.empty?
      if has_gaps && !has_commit
        unresolved << "#{scope} review[#{idx}] has gaps without closing commit"
      end
    end

    unresolved
  end
end

exit Verifier.new(ARGV).run
RUBY
