#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 1 ]; then
  cat >&2 <<'USAGE'
Usage:
  tools/roadmap/verify_done.sh <phase-yaml> [<phase-yaml> ...]

Behavior:
  - For each targeted phase:
    - skips validation when done!=true
    - fails if any item has done!=true
    - fails if any done item is missing acceptance checks (`verification`) or acceptance evidence (`reviews[*].commit`)
    - fails if any unresolved reviews.gaps exist (phase or item level)
    - fails when phase index evidence marker is missing
    - fails when phase index evidence marker is present but unchecked
    - fails when phase is not done but index evidence marker is checked

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
    @skipped = 0
  end

  def run
    @paths.each { |path| verify_phase(path) }

    if @failures.any?
      @failures.each { |line| warn(line) }
      warn("roadmap verification failed")
      return 1
    end

    puts("roadmap verification passed (#{@checked} phase#{@checked == 1 ? "" : "s"} checked, #{@skipped} skipped)")
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

    phase_name = File.basename(path, ".yaml")
    done = data["done"] == true
    unless done
      @skipped += 1
      verify_not_done_evidence_marker(path, phase_name)
      return
    end

    @checked += 1
    unresolved = []
    incomplete_items = []
    acceptance_gaps = []

    unresolved.concat(find_unresolved_reviews(data["reviews"], "phase"))

    items = data["items"]
    if items.is_a?(Array)
      items.each_with_index do |item, idx|
        next unless item.is_a?(Hash)

        label = item["label"].to_s.strip
        marker = label.empty? ? "item[#{idx}]" : "item #{label}"
        incomplete_items << marker unless item["done"] == true
        acceptance_gaps.concat(find_acceptance_gaps(item, marker))
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

    if acceptance_gaps.any?
      @failures << "error: missing acceptance completion/evidence in #{path}"
      acceptance_gaps.each { |msg| @failures << "  - #{msg}" }
    end

    verify_evidence_marker(path, phase_name)
  rescue Psych::SyntaxError => e
    @failures << "error: YAML parse failure in #{path}: #{e.message.lines.first.to_s.strip}"
  end

  def verify_evidence_marker(path, phase_name)
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
  end

  def verify_not_done_evidence_marker(path, phase_name)
    index_path = File.join(File.dirname(path), "index.md")
    evidence_marker = "evidence:#{phase_name}"

    return unless File.file?(index_path)

    index_text = File.read(index_path)
    lines_with_marker = index_text.each_line.select { |line| line.include?(evidence_marker) }
    return if lines_with_marker.empty?

    has_checked_entry = lines_with_marker.any? { |line| line.match?(/^\s*-\s*\[[xX]\]/) }
    if has_checked_entry
      @failures << "error: evidence marker '#{evidence_marker}' is checked while #{path} has done!=true"
    end
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

  def find_acceptance_gaps(item, scope)
    return [] unless item.is_a?(Hash)
    return [] unless item["done"] == true

    gaps = []
    verification = item["verification"]
    if !verification.is_a?(Array) || verification.empty?
      gaps << "#{scope} has done=true but missing verification acceptance checks"
    end

    reviews = item["reviews"]
    if !reviews.is_a?(Array) || reviews.empty?
      gaps << "#{scope} has done=true but missing reviews acceptance evidence"
      return gaps
    end

    review_commits = []
    reviews.each_with_index do |review, idx|
      next unless review.is_a?(Hash)

      commit = review["commit"].to_s.strip
      if commit.empty?
        gaps << "#{scope} review[#{idx}] missing commit acceptance evidence"
        next
      end
      review_commits << commit
    end

    if review_commits.empty?
      gaps << "#{scope} has done=true but no review commit acceptance evidence"
    end

    gaps
  end
end

exit Verifier.new(ARGV).run
RUBY
