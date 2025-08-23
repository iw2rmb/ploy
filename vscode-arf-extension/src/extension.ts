import * as vscode from 'vscode';
import { ARFRecipeProvider } from './providers/recipeProvider';
import { ARFPreviewProvider } from './providers/previewProvider';
import { ARFValidationProvider } from './providers/validationProvider';
import { ARFController } from './controllers/arfController';
import { ARFRecipeTreeProvider } from './providers/treeProvider';
import { LLMRecipeGenerator } from './generators/llmGenerator';
import { DryRunExecutor } from './executors/dryRunExecutor';

export function activate(context: vscode.ExtensionContext) {
    console.log('ARF Extension is now active!');

    // Initialize components
    const arfController = new ARFController();
    const recipeProvider = new ARFRecipeProvider();
    const previewProvider = new ARFPreviewProvider(context.extensionUri);
    const validationProvider = new ARFValidationProvider(arfController);
    const treeProvider = new ARFRecipeTreeProvider();
    const llmGenerator = new LLMRecipeGenerator(arfController);
    const dryRunExecutor = new DryRunExecutor(arfController);

    // Register tree view
    const recipeTreeView = vscode.window.createTreeView('arfRecipes', {
        treeDataProvider: treeProvider,
        canSelectMany: false
    });

    // Register webview providers
    context.subscriptions.push(
        vscode.window.registerWebviewViewProvider('arf.recipePreview', previewProvider)
    );

    // Register language providers
    context.subscriptions.push(
        vscode.languages.registerCompletionItemProvider(
            { language: 'arf-recipe' },
            recipeProvider,
            '.', '"', "'", ' '
        )
    );

    context.subscriptions.push(
        vscode.languages.registerHoverProvider(
            { language: 'arf-recipe' },
            recipeProvider
        )
    );

    context.subscriptions.push(
        vscode.languages.registerDocumentFormattingEditProvider(
            { language: 'arf-recipe' },
            recipeProvider
        )
    );

    // Register diagnostic provider for real-time validation
    const diagnosticCollection = vscode.languages.createDiagnosticCollection('arf-recipe');
    context.subscriptions.push(diagnosticCollection);

    // Auto-validate on document change
    const validateDocument = async (document: vscode.TextDocument) => {
        if (document.languageId === 'arf-recipe') {
            const diagnostics = await validationProvider.validateDocument(document);
            diagnosticCollection.set(document.uri, diagnostics);
        }
    };

    // Register document change handlers
    context.subscriptions.push(
        vscode.workspace.onDidChangeTextDocument(e => {
            if (vscode.workspace.getConfiguration('arf').get('validation.realTime')) {
                validateDocument(e.document);
            }
        })
    );

    context.subscriptions.push(
        vscode.workspace.onDidOpenTextDocument(validateDocument)
    );

    // Register commands
    context.subscriptions.push(
        vscode.commands.registerCommand('arf.createRecipe', async (uri?: vscode.Uri) => {
            const targetFolder = uri || vscode.workspace.workspaceFolders?.[0]?.uri;
            if (!targetFolder) {
                vscode.window.showErrorMessage('No workspace folder available');
                return;
            }

            const recipeName = await vscode.window.showInputBox({
                prompt: 'Enter recipe name',
                placeHolder: 'my-transformation-recipe'
            });

            if (!recipeName) {
                return;
            }

            const template = await vscode.window.showQuickPick([
                { label: 'Java OpenRewrite Recipe', value: 'java-openrewrite' },
                { label: 'JavaScript/TypeScript Recipe', value: 'js-ts' },
                { label: 'Python Recipe', value: 'python' },
                { label: 'Go Recipe', value: 'go' },
                { label: 'Multi-language Recipe', value: 'multi-lang' },
                { label: 'LLM-enhanced Recipe', value: 'llm-enhanced' },
                { label: 'Empty Recipe', value: 'empty' }
            ], {
                placeHolder: 'Select recipe template'
            });

            if (!template) {
                return;
            }

            await recipeProvider.createRecipeFromTemplate(
                targetFolder, 
                recipeName, 
                template.value
            );
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('arf.validateRecipe', async () => {
            const editor = vscode.window.activeTextEditor;
            if (!editor || editor.document.languageId !== 'arf-recipe') {
                vscode.window.showWarningMessage('Please open an ARF recipe file');
                return;
            }

            const diagnostics = await validationProvider.validateDocument(editor.document);
            diagnosticCollection.set(editor.document.uri, diagnostics);

            if (diagnostics.length === 0) {
                vscode.window.showInformationMessage('✅ Recipe validation passed');
            } else {
                const errorCount = diagnostics.filter(d => d.severity === vscode.DiagnosticSeverity.Error).length;
                const warningCount = diagnostics.filter(d => d.severity === vscode.DiagnosticSeverity.Warning).length;
                
                vscode.window.showWarningMessage(
                    `Recipe validation found ${errorCount} errors and ${warningCount} warnings`
                );
            }
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('arf.previewTransformation', async () => {
            const editor = vscode.window.activeTextEditor;
            if (!editor || editor.document.languageId !== 'arf-recipe') {
                vscode.window.showWarningMessage('Please open an ARF recipe file');
                return;
            }

            // Get target code for preview
            const targetFile = await vscode.window.showOpenDialog({
                canSelectFiles: true,
                canSelectFolders: false,
                canSelectMany: false,
                filters: {
                    'Code Files': ['java', 'js', 'ts', 'py', 'go', 'rs'],
                    'All Files': ['*']
                }
            });

            if (!targetFile || targetFile.length === 0) {
                return;
            }

            await previewProvider.showPreview(editor.document, targetFile[0]);
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('arf.dryRun', async () => {
            const editor = vscode.window.activeTextEditor;
            if (!editor || editor.document.languageId !== 'arf-recipe') {
                vscode.window.showWarningMessage('Please open an ARF recipe file');
                return;
            }

            // Select target directory or file
            const target = await vscode.window.showOpenDialog({
                canSelectFiles: true,
                canSelectFolders: true,
                canSelectMany: false,
                openLabel: 'Select Target for Dry Run'
            });

            if (!target || target.length === 0) {
                return;
            }

            vscode.window.withProgress({
                location: vscode.ProgressLocation.Notification,
                title: 'Running ARF Recipe Dry Run...',
                cancellable: true
            }, async (progress, token) => {
                try {
                    const result = await dryRunExecutor.executeDryRun(
                        editor.document, 
                        target[0], 
                        progress, 
                        token
                    );
                    
                    await dryRunExecutor.showDryRunResults(result);
                } catch (error) {
                    vscode.window.showErrorMessage(`Dry run failed: ${error}`);
                }
            });
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('arf.testRecipe', async () => {
            const editor = vscode.window.activeTextEditor;
            if (!editor || editor.document.languageId !== 'arf-recipe') {
                vscode.window.showWarningMessage('Please open an ARF recipe file');
                return;
            }

            // Create test task
            const task = new vscode.Task(
                { type: 'arf', recipe: editor.document.fileName },
                vscode.TaskScope.Workspace,
                'Test ARF Recipe',
                'arf',
                new vscode.ShellExecution('echo "Testing ARF Recipe..."')
            );

            await vscode.tasks.executeTask(task);
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('arf.generateFromLLM', async () => {
            // Get current error context from open files
            const errorDescription = await vscode.window.showInputBox({
                prompt: 'Describe the error or transformation needed',
                placeHolder: 'e.g., "Java compilation error: cannot find symbol HttpServletRequest"',
                multiline: true
            });

            if (!errorDescription) {
                return;
            }

            const language = await vscode.window.showQuickPick([
                'java', 'javascript', 'typescript', 'python', 'go', 'rust'
            ], {
                placeHolder: 'Select target language'
            });

            if (!language) {
                return;
            }

            const framework = await vscode.window.showInputBox({
                prompt: 'Enter framework/library (optional)',
                placeHolder: 'e.g., spring-boot, react, django'
            });

            vscode.window.withProgress({
                location: vscode.ProgressLocation.Notification,
                title: 'Generating recipe with LLM...',
                cancellable: false
            }, async (progress) => {
                try {
                    const recipe = await llmGenerator.generateRecipe({
                        errorDescription,
                        language,
                        framework: framework || undefined,
                        progress
                    });

                    await llmGenerator.openGeneratedRecipe(recipe);
                } catch (error) {
                    vscode.window.showErrorMessage(`LLM generation failed: ${error}`);
                }
            });
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('arf.connectToController', async () => {
            const currentUrl = vscode.workspace.getConfiguration('arf').get<string>('controllerUrl');
            
            const newUrl = await vscode.window.showInputBox({
                prompt: 'Enter ARF Controller URL',
                value: currentUrl,
                placeHolder: 'http://localhost:8081/v1'
            });

            if (!newUrl) {
                return;
            }

            // Test connection
            try {
                await arfController.testConnection(newUrl);
                
                await vscode.workspace.getConfiguration('arf').update(
                    'controllerUrl', 
                    newUrl, 
                    vscode.ConfigurationTarget.Workspace
                );

                vscode.window.showInformationMessage('✅ Connected to ARF Controller');
            } catch (error) {
                vscode.window.showErrorMessage(`Connection failed: ${error}`);
            }
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('arf.deployRecipe', async () => {
            const editor = vscode.window.activeTextEditor;
            if (!editor || editor.document.languageId !== 'arf-recipe') {
                vscode.window.showWarningMessage('Please open an ARF recipe file');
                return;
            }

            vscode.window.withProgress({
                location: vscode.ProgressLocation.Notification,
                title: 'Deploying recipe to controller...',
                cancellable: false
            }, async (progress) => {
                try {
                    await arfController.deployRecipe(editor.document, progress);
                    vscode.window.showInformationMessage('✅ Recipe deployed successfully');
                } catch (error) {
                    vscode.window.showErrorMessage(`Deployment failed: ${error}`);
                }
            });
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('arf.openDocumentation', () => {
            vscode.env.openExternal(vscode.Uri.parse('https://github.com/iw2rmb/ploy/blob/main/docs/WASM.md'));
        })
    );

    // Status bar items
    const statusBarItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
    statusBarItem.text = '$(tools) ARF';
    statusBarItem.tooltip = 'ARF Toolkit - Click to open documentation';
    statusBarItem.command = 'arf.openDocumentation';
    statusBarItem.show();
    context.subscriptions.push(statusBarItem);

    // Initialize workspace scanning for recipes
    if (vscode.workspace.workspaceFolders) {
        treeProvider.refresh();
    }

    console.log('ARF Extension initialization complete');
}

export function deactivate() {
    console.log('ARF Extension deactivated');
}