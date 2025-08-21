/**
 * Automatic Agent Selection System for Ploy Project
 * Analyzes task context and automatically selects appropriate specialized agent
 */

const fs = require('fs');
const path = require('path');

class PloyAgentSelector {
    constructor() {
        this.config = this.loadConfig();
    }

    loadConfig() {
        const configPath = path.join(__dirname, 'agents.json');
        if (fs.existsSync(configPath)) {
            return JSON.parse(fs.readFileSync(configPath, 'utf8'));
        }
        throw new Error('Agent configuration file not found');
    }

    /**
     * Analyze task and select appropriate agent
     * @param {string} taskDescription - The task description
     * @param {string[]} filePaths - Array of file paths involved
     * @param {string[]} keywords - Additional keywords/context
     * @returns {Object} Selected agent info
     */
    selectAgent(taskDescription, filePaths = [], keywords = []) {
        const scores = {};
        const agents = this.config.agents;

        // Initialize scores
        Object.keys(agents).forEach(agentId => {
            scores[agentId] = 0;
        });

        // Score based on priority keywords
        const allText = `${taskDescription} ${keywords.join(' ')}`.toLowerCase();
        Object.entries(this.config.selection_rules.priority_keywords).forEach(([keyword, agentId]) => {
            if (allText.includes(keyword.toLowerCase())) {
                scores[agentId] = (scores[agentId] || 0) + 3;
            }
        });

        // Score based on trigger words
        Object.entries(agents).forEach(([agentId, agent]) => {
            agent.triggers.forEach(trigger => {
                if (allText.includes(trigger.toLowerCase())) {
                    scores[agentId] = (scores[agentId] || 0) + 2;
                }
            });
        });

        // Score based on file paths
        filePaths.forEach(filePath => {
            Object.entries(this.config.selection_rules.file_patterns).forEach(([pattern, agentId]) => {
                if (filePath.includes(pattern)) {
                    scores[agentId] = (scores[agentId] || 0) + 2;
                }
            });
        });

        // Find best match
        const bestAgent = Object.entries(scores).reduce((best, [agentId, score]) => {
            return score > best.score ? { agentId, score } : best;
        }, { agentId: null, score: 0 });

        // Check confidence threshold
        const confidence = bestAgent.score / 10; // Normalize to 0-1 scale
        
        if (confidence >= this.config.auto_selection.confidence_threshold) {
            return {
                agentId: bestAgent.agentId,
                agent: agents[bestAgent.agentId],
                confidence: confidence,
                reasoning: this.generateReasoning(bestAgent.agentId, taskDescription, filePaths)
            };
        }

        // Fallback to general purpose
        return {
            agentId: this.config.auto_selection.fallback_agent,
            agent: null,
            confidence: 0,
            reasoning: "No specialized agent matched with sufficient confidence"
        };
    }

    generateReasoning(agentId, taskDescription, filePaths) {
        const agent = this.config.agents[agentId];
        const reasons = [];

        // Check what triggered the selection
        const allText = taskDescription.toLowerCase();
        
        // Priority keywords
        Object.entries(this.config.selection_rules.priority_keywords).forEach(([keyword, targetAgent]) => {
            if (targetAgent === agentId && allText.includes(keyword.toLowerCase())) {
                reasons.push(`Contains priority keyword: "${keyword}"`);
            }
        });

        // Trigger words
        agent.triggers.forEach(trigger => {
            if (allText.includes(trigger.toLowerCase())) {
                reasons.push(`Matches trigger: "${trigger}"`);
            }
        });

        // File patterns
        filePaths.forEach(filePath => {
            Object.entries(this.config.selection_rules.file_patterns).forEach(([pattern, targetAgent]) => {
                if (targetAgent === agentId && filePath.includes(pattern)) {
                    reasons.push(`File path matches: "${pattern}"`);
                }
            });
        });

        return reasons.join(', ');
    }

    /**
     * Get agent recommendation for Claude Code Task tool
     * @param {string} taskDescription 
     * @param {string[]} filePaths 
     * @param {string[]} keywords 
     * @returns {string} Agent selection result
     */
    getRecommendation(taskDescription, filePaths = [], keywords = []) {
        const selection = this.selectAgent(taskDescription, filePaths, keywords);
        
        if (selection.agentId === this.config.auto_selection.fallback_agent) {
            return {
                useTask: false,
                recommendation: "Use standard tools - no specialized agent needed"
            };
        }

        return {
            useTask: true,
            agentType: selection.agentId,
            confidence: selection.confidence,
            reasoning: selection.reasoning,
            description: selection.agent.description,
            tools: selection.agent.tools,
            expertise: selection.agent.expertise
        };
    }
}

// Example usage
if (require.main === module) {
    const selector = new PloyAgentSelector();
    
    // Test cases
    const testCases = [
        {
            task: "Add certificate management to domain API endpoints",
            files: ["controller/server/server.go", "internal/cli/domains/handler.go"],
            keywords: ["ACME", "SSL"]
        },
        {
            task: "Analyze Go application for optimal lane selection", 
            files: ["tools/lane-pick/analyzer.go"],
            keywords: ["performance", "unikernel"]
        },
        {
            task: "Deploy updated controller to VPS using Nomad",
            files: ["iac/dev/playbooks/main.yml"],
            keywords: ["deployment", "infrastructure"]
        }
    ];

    testCases.forEach((testCase, i) => {
        console.log(`\n=== Test Case ${i + 1} ===`);
        console.log(`Task: ${testCase.task}`);
        const recommendation = selector.getRecommendation(testCase.task, testCase.files, testCase.keywords);
        console.log('Recommendation:', JSON.stringify(recommendation, null, 2));
    });
}

module.exports = PloyAgentSelector;